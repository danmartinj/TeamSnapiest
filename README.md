# TeamSnapiest
A simple Go API client for TeamSnap that also provides a user command line tool to perform some limited queries.

## Description

TeamSnapiest, inspired from the python TeamSnappier tool is built to mainly provide features to show all teams a user is assigned to, list members of a specific team, and show all up and coming events across all teams. The library can do more and it can easily be extended. Currently, the library only supports read capabilities.

## Requirements

1. Golang installed
2. A completed config.ini file by registering an app (use urn:ietf:wg:oauth:2.0:oob for the redirect URL) using the [TeamSnap Application Registration Page](https://auth.teamsnap.com/)

## Once built command line tool supports

- Show current user details
- Show active teams user is apart of
- Show all up and coming events across all teams
- List members (name and email) of specified team
- export all up and coming events across all teams to CSV
