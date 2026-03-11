package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Helper to read a single line of input from the console
func readInput(prompt string) string {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return strings.TrimSpace(scanner.Text())
}

func main() {
	// 1. Initialize Client
	client, err := NewTeamSnappiestFromConfig("config.ini")
	if err != nil {
		fmt.Printf("Error initializing client: %v\n", err)
		os.Exit(1)
	}

	// 2. Fetch User Details (We need this for the ID)
	fmt.Println("Connecting to TeamSnap...")
	meList, err := client.FindMe()
	if err != nil || len(meList) == 0 {
		fmt.Printf("Error finding user details: %v\n", err)
		os.Exit(1)
	}

	// Store User Details
	me := meList[0]
	myUserID := fmt.Sprintf("%v", me["id"])
	firstName := fmt.Sprintf("%v", me["first_name"])
	lastName := fmt.Sprintf("%v", me["last_name"])

	fmt.Printf("\nWelcome, %s %s! (User ID: %s)\n", firstName, lastName, myUserID)

	// 3. Main Application Loop
	for {
		printMenu()
		choice := readInput("\nEnter your choice (1-6): ")

		switch choice {
		case "1":
			showUserDetails(me)
		case "2":
			showActiveTeams(client, myUserID)
		case "3":
			showUpcomingEvents(client, myUserID)
		case "4":
			listTeamMembers(client, myUserID)
		case "5":
			exportEventsToCSV(client, myUserID)
		case "6":
			fmt.Println("Goodbye!")
			return
		default:
			fmt.Println("Invalid choice, please try again.")
		}

		fmt.Println("\nPress Enter to continue...")
		bufio.NewScanner(os.Stdin).Scan() // Wait for user to press Enter
	}
}

func printMenu() {
	fmt.Println("\n=================================")
	fmt.Println("   TeamSnap CLI Utility")
	fmt.Println("=================================")
	fmt.Println("1. Show My User Details")
	fmt.Println("2. Show My Active Teams")
	fmt.Println("3. Show Upcoming Events (All Teams)")
	fmt.Println("4. List Members of a Specific Team")
	fmt.Println("5. Export Upcoming Events to CSV")
	fmt.Println("6. Exit")
	fmt.Println("=================================")
}

// --- Action 1: User Details ---
func showUserDetails(me map[string]interface{}) {
	fmt.Println("\n--- User Details ---")
	fmt.Printf("ID:         %v\n", me["id"])
	fmt.Printf("Name:       %v %v\n", me["first_name"], me["last_name"])
	fmt.Printf("Email:      %v\n", me["email"])
	fmt.Printf("Username:   %v\n", me["username"])
	if tz, ok := me["time_zone"]; ok {
		fmt.Printf("Time Zone:  %v\n", tz)
	}
}

// --- Action 2: Active Teams ---
func showActiveTeams(client *TeamSnappiest, userID string) {
	fmt.Println("\n--- Fetching Active Teams ---")
	teams, err := client.ListTeams(userID)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if len(teams) == 0 {
		fmt.Println("No active teams found.")
		return
	}

	fmt.Printf("%-15s | %-30s | %s\n", "ID", "Name", "Sport")
	fmt.Println("---------------------------------------------------------------")
	for _, team := range teams {
		id := fmt.Sprintf("%v", team["id"])
		name := fmt.Sprintf("%v", team["name"])
		sport := fmt.Sprintf("%v", team["sport_name"])
		fmt.Printf("%-15s | %-30s | %s\n", id, name, sport)
	}
}

// Helper function to format an address from a location map
func formatAddress(loc map[string]interface{}) string {
	// Try the pre-formatted address first
	address := fmt.Sprintf("%v", loc["address"])
	if address != "" && address != "<nil>" {
		return address
	}

	// Fallback to components
	var parts []string
	if v := fmt.Sprintf("%v", loc["address_line_1"]); v != "" && v != "<nil>" {
		parts = append(parts, v)
	}
	if v := fmt.Sprintf("%v", loc["city"]); v != "" && v != "<nil>" {
		parts = append(parts, v)
	}
	if v := fmt.Sprintf("%v", loc["state"]); v != "" && v != "<nil>" {
		parts = append(parts, v)
	}
	return strings.Join(parts, ", ")
}

// Helper function to get upcoming events logic
// Used by both showUpcomingEvents and exportEventsToCSV
func getUpcomingEventsLogic(client *TeamSnappiest, userID string) ([]map[string]interface{}, error) {
	// Get all teams first
	teams, err := client.ListTeams(userID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var allRelevantEvents []map[string]interface{}

	// Cache for location addresses: TeamID -> LocationID -> Address
	teamLocationCache := make(map[string]map[string]string)

	// Load the specific location (handles EST vs EDT automatically)
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		// Fallback if timezone db isn't found (rare on local machines)
		fmt.Println("Error loading timezone, defaulting to UTC-5")
		loc = time.FixedZone("EST", -5*60*60)
	}

	for _, team := range teams {
		teamID := fmt.Sprintf("%v", team["id"])
		teamName := fmt.Sprintf("%v", team["name"])

		// 1. Populate Location Cache for this team (Using generic listResources to hit /locations/search)
		if _, ok := teamLocationCache[teamID]; !ok {
			teamLocationCache[teamID] = make(map[string]string)
			locs, err := client.listResources("/locations/search", map[string]string{"team_id": teamID}, "")
			if err == nil {
				for _, locData := range locs { // Renamed 'loc' to 'locData' to avoid conflict with time.Location 'loc'
					locID := fmt.Sprintf("%v", locData["id"])
					teamLocationCache[teamID][locID] = formatAddress(locData)
				}
			}
		}

		// 2. Fetch Events
		events, err := client.ListEvents(userID, teamID)
		if err != nil {
			fmt.Printf("Warning: Error fetching events for team %s: %v\n", teamName, err)
			continue
		}

		for _, event := range events {
			startStr, ok := event["start_date"].(string)
			if !ok || startStr == "" {
				continue
			}

			eventTime, err := time.Parse(time.RFC3339, startStr)
			if err != nil {
				continue
			}

			// Adjust Time to EST/EDT
			adjustedEventTime := eventTime.In(loc)

			// Filter: Recent/Future
			if adjustedEventTime.After(now.Add(-12 * time.Hour)) {
				event["team_name"] = teamName
				event["adjusted_start_date"] = adjustedEventTime.Format(time.RFC3339)

				// Lookup Address
				locID := fmt.Sprintf("%v", event["location_id"])
				if addr, found := teamLocationCache[teamID][locID]; found {
					event["resolved_address"] = addr
				} else {
					event["resolved_address"] = "N/A"
				}

				allRelevantEvents = append(allRelevantEvents, event)
			}
		}
	}

	// Sort by date
	sort.Slice(allRelevantEvents, func(i, j int) bool {
		timeA, _ := time.Parse(time.RFC3339, allRelevantEvents[i]["adjusted_start_date"].(string))
		timeB, _ := time.Parse(time.RFC3339, allRelevantEvents[j]["adjusted_start_date"].(string))
		return timeA.Before(timeB)
	})

	return allRelevantEvents, nil
}

// --- Action 3: Show Upcoming Events (Display) ---
func showUpcomingEvents(client *TeamSnappiest, userID string) {
	fmt.Println("\n--- Fetching Upcoming Events... ---")

	events, err := getUpcomingEventsLogic(client, userID)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if len(events) == 0 {
		fmt.Println("No upcoming events found.")
		return
	}

	fmt.Println("\n--- UPCOMING EVENTS ---")
	// Header updated to match the widths of the data columns
	fmt.Printf("%-15s | %-20s | %-20s | %-20s | %-55s | %-20s | %s\n",
		"Date (-5h)", "Team", "Event", "Location", "Address", "Opponent", "Notes")
	fmt.Println(strings.Repeat("-", 190))

	for _, event := range events {
		adjustedEventTime, _ := time.Parse(time.RFC3339, event["adjusted_start_date"].(string))

		s := func(k string) string { return fmt.Sprintf("%v", event[k]) }

		// Using the requested format:
		// %-15s | %-20.20s | %-20.20s | %-20.20s | %-55.55s | %-20.20s | %.200s
		fmt.Printf("%-15s | %-20.20s | %-20.20s | %-20.20s | %-55.55s | %-20.20s | %.200s\n",
			adjustedEventTime.Format("06-01-02 15:04"),
			truncateString(s("team_name"), 20),
			truncateString(s("name"), 20),
			truncateString(s("location_name"), 20),
			truncateString(s("resolved_address"), 55),
			truncateString(s("opponent_name"), 20),
			truncateString(s("notes"), 200),
		)
	}
}

// --- Action 5: Export to CSV ---
func exportEventsToCSV(client *TeamSnappiest, userID string) {
	fmt.Println("\n--- Generating CSV Export... ---")

	events, err := getUpcomingEventsLogic(client, userID)
	if err != nil {
		fmt.Printf("Error fetching events: %v\n", err)
		return
	}

	if len(events) == 0 {
		fmt.Println("No events to export.")
		return
	}

	filename := "upcoming_events.csv"
	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Error creating file: %v\n", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 1. Write Header
	header := []string{"Date", "Time", "Team", "Event Name", "Location Name", "Address", "Opponent", "Notes", "Team ID", "Event ID"}
	if err := writer.Write(header); err != nil {
		fmt.Printf("Error writing header: %v\n", err)
		return
	}

	// 2. Write Data Rows
	count := 0
	for _, event := range events {
		adjustedEventTime, _ := time.Parse(time.RFC3339, event["adjusted_start_date"].(string))
		s := func(k string) string { return fmt.Sprintf("%v", event[k]) }

		row := []string{
			adjustedEventTime.Format("2006-01-02"), // Date
			adjustedEventTime.Format("15:04"),      // Time
			s("team_name"),
			s("name"),
			s("location_name"),
			s("resolved_address"),
			s("opponent_name"),
			s("notes"),
			s("team_id"),
			s("id"),
		}

		if err := writer.Write(row); err != nil {
			fmt.Printf("Error writing row: %v\n", err)
			return
		}
		count++
	}

	fmt.Printf("Successfully exported %d events to '%s'\n", count, filename)
}

// --- Action 4: List Members ---
func listTeamMembers(client *TeamSnappiest, userID string) {
	showActiveTeams(client, userID)

	teamID := readInput("\nEnter the Team ID to view members: ")
	if teamID == "" {
		fmt.Println("Operation cancelled.")
		return
	}

	fmt.Printf("\n--- Fetching Members for Team ID: %s ---\n", teamID)
	members, err := client.ListMembers(teamID)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if len(members) == 0 {
		fmt.Println("No members found.")
		return
	}

	// Use the PrintMembers function from client.go (assuming it's still in main package)
	PrintMembers(members)
}

// Helper function to truncate strings for display
func truncateString(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s
	}
	return s[:maxLen-3] + "..."
}
