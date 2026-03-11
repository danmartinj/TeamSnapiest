package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	// Added for converting float64 to string for IDs
	"gopkg.in/ini.v1"
)

const (
	baseURL      = "https://api.teamsnap.com/v3"
	acceptHeader = "application/vnd.collection+json"
)

// DataItem represents a key-value pair in the TeamSnap API's Collection+JSON format.
type DataItem struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"` // Value can be various types (string, int, bool, float64, etc.)
}

// CollectionItem represents an item within the 'items' array of a Collection+JSON response.
type CollectionItem struct {
	Data []DataItem `json:"data"`
}

// CollectionRoot is the top-level structure for most TeamSnap API Collection+JSON responses.
type CollectionRoot struct {
	Collection struct {
		Items []CollectionItem `json:"items"`
	} `json:"collection"`
}

// TeamSnappiest struct holds the HTTP client, access token, and default headers.
type TeamSnappiest struct {
	httpClient  *http.Client
	accessToken string
	headers     http.Header
}

// NewTeamSnappiestFromConfig creates and initializes a new TeamSnappiest client.
// It reads the access token from the specified config file.
func NewTeamSnappiestFromConfig(cfgPath string) (*TeamSnappiest, error) {
	cfg, err := ini.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file '%s': %w", cfgPath, err)
	}
	token := cfg.Section("api").Key("access_token").String()
	if token == "" {
		return nil, errors.New("access_token not found in config.ini under [api] section")
	}
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+token)
	h.Set("Accept", acceptHeader)
	return &TeamSnappiest{
		httpClient:  &http.Client{},
		accessToken: token,
		headers:     h,
	}, nil
}

// doRequest is a helper method to perform HTTP GET/POST/DELETE requests and handle common headers/errors.
// It returns the raw response body as []byte, the HTTP status code, and an error.
func (c *TeamSnappiest) doRequest(method, rawurl string, params map[string]string, body interface{}) ([]byte, int, error) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid URL '%s': %w", rawurl, err)
	}

	// Add query parameters for GET requests
	if params != nil && method == http.MethodGet {
		q := u.Query()
		for k, v := range params {
			if v != "" { // Only add non-empty parameters
				q.Set(k, v)
			}
		}
		u.RawQuery = q.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, u.String(), reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	for k, vs := range c.headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Send the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}

	return b, resp.StatusCode, nil
}

// flattenCollection converts a raw Collection+JSON response body into a slice of flattened maps.
// Each map represents an item with key-value pairs (field name to field value).
func flattenCollection(body []byte) ([]map[string]interface{}, error) {
	var root CollectionRoot
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("failed to unmarshal collection JSON: %w", err)
	}

	out := make([]map[string]interface{}, 0, len(root.Collection.Items))
	for _, item := range root.Collection.Items {
		m := make(map[string]interface{})
		for _, d := range item.Data {
			m[d.Name] = d.Value
		}
		out = append(out, m)
	}
	return out, nil
}

// findMe fetches the current authenticated user's details.
func (c *TeamSnappiest) FindMe() ([]map[string]interface{}, error) {
	body, status, err := c.doRequest(http.MethodGet, baseURL+"/me", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("FindMe request failed: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("FindMe failed with status %d: %s", status, string(body))
	}
	fmt.Println("find_me() was successful!")
	return flattenCollection(body)
}

// GetURL performs a GET request to a specified URL and returns the raw JSON response.
func (c *TeamSnappiest) GetURL(urlStr string) (map[string]interface{}, error) {
	body, status, err := c.doRequest(http.MethodGet, urlStr, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("GetURL request failed: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("GetURL failed with status %d: %s", status, string(body))
	}
	var out map[string]interface{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GetURL response: %w", err)
	}
	fmt.Println("get_url() was successful!")
	return out, nil
}

// listResources is a generic helper for listing various resources using a search endpoint.
func (c *TeamSnappiest) listResources(path string, params map[string]string, successMsg string) ([]map[string]interface{}, error) {
	fullPath := baseURL + path
	body, status, err := c.doRequest(http.MethodGet, fullPath, params, nil)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", path, err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("failed to list resources from %s (status %d): %s", path, status, string(body))
	}
	if successMsg != "" {
		fmt.Printf("%s was successful!\n", successMsg)
	}
	return flattenCollection(body)
}

// ListAssignments lists assignments for a given team ID.
func (c *TeamSnappiest) ListAssignments(teamID string) ([]map[string]interface{}, error) {
	return c.listResources("/assignments/search", map[string]string{"team_id": teamID}, "list_assignments()")
}

// ListMembers lists members for a given team ID.
func (c *TeamSnappiest) ListMembers(teamID string) ([]map[string]interface{}, error) {
	return c.listResources("/members/search", map[string]string{"team_id": teamID}, "list_members()")
}

// ListDivisions lists divisions by ID.
func (c *TeamSnappiest) ListDivisions(divisionID string) ([]map[string]interface{}, error) {
	return c.listResources("/divisions/search", map[string]string{"id": divisionID}, "list_divisions()")
}

// ListDivisionLocations lists division locations by division ID.
func (c *TeamSnappiest) ListDivisionLocations(divisionID string) ([]map[string]interface{}, error) {
	return c.listResources("/division_locations/search", map[string]string{"division_id": divisionID}, "list_division_locations()")
}

// ListTeams lists ONLY ACTIVE teams for a given user ID.
func (c *TeamSnappiest) ListTeams(userID string) ([]map[string]interface{}, error) {
	fmt.Printf("Fetching active teams for UserID: %s\n", userID)
	return c.listResources("/teams/active", map[string]string{"user_id": userID}, "list_active_teams()")
}

// ListEvents lists events for a given user and/or team ID.
func (c *TeamSnappiest) ListEvents(userID, teamID string) ([]map[string]interface{}, error) {
	params := make(map[string]string)
	if userID != "" {
		params["user_id"] = userID
	}
	if teamID != "" {
		params["team_id"] = teamID
	}
	return c.listResources("/events/search", params, "list_events()")
}

// PrintList prints a slice of maps (objects), optionally filtering by specified variables.
func PrintList(list []map[string]interface{}, variables []string) {
	fmt.Println("************************")
	if len(variables) > 0 {
		for _, item := range list {
			for _, v := range variables {
				if val, ok := item[v]; ok {
					fmt.Printf("%s: %v\n", v, val)
				} else {
					fmt.Printf("%s: (not found)\n", v)
				}
			}
			fmt.Println("---------------------------------------------")
		}
	} else { // Print all fields if no specific variables are requested
		for _, m := range list {
			for k, v := range m {
				fmt.Printf("%s: %v\n", k, v)
			}
			fmt.Println("---------------------------------------------")
		}
	}
}

// PrintMembers specifically prints formatted member details.
func PrintMembers(memberList []map[string]interface{}) {
	fmt.Println("Printing Members:")
	fmt.Println("************************")

	for _, member := range memberList {
		firstName, _ := member["first_name"].(string)
		lastName, _ := member["last_name"].(string)

		// email_addresses in TeamSnap API can be complex. For simplicity,
		// we'll try to represent it. If it's a slice of DataItem, this will print.
		// A more robust solution might unmarshal it into a specific email address struct.
		emailAddresses := fmt.Sprintf("%v", member["email_addresses"])

		fmt.Printf("First Name: %s\n", firstName)
		fmt.Printf("Last Name: %s\n", lastName)
		fmt.Printf("Email address: %s\n\n", emailAddresses)
	}
}
