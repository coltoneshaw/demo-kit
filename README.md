# Mattermost Demo Kit

This repository contains tools for setting up and configuring a Mattermost demo environment with integrated applications.

## Configuration

### User and Team Configuration

You can customize the users and teams created by the Mattermost setup tool by editing the `config.json` file in the root directory.

Example `config.json`:

```json
{
  "users": [
    {
      "username": "sysadmin",
      "email": "sysadmin@example.com",
      "password": "Testpassword123!",
      "nickname": "System Administrator",
      "isSystemAdmin": true,
      "teams": ["test", "marketing"]
    },
    {
      "username": "user-1",
      "email": "user-1@example.com",
      "password": "Testpassword123!",
      "nickname": "Regular User",
      "isSystemAdmin": false,
      "teams": ["test"]
    }
  ],
  "teams": {
    "test": {
      "name": "test",
      "displayName": "Test Team",
      "description": "Team for testing",
      "type": "O",
      "channels": [
        {
          "name": "announcements",
          "displayName": "Announcements",
          "purpose": "Important team announcements",
          "header": "📢 Official announcements channel",
          "type": "O",
          "members": ["sysadmin", "user-1"]
        },
        {
          "name": "development",
          "displayName": "Development",
          "purpose": "Development discussions",
          "type": "O",
          "members": ["sysadmin", "user-1"]
        },
        {
          "name": "private-notes",
          "displayName": "Private Notes",
          "purpose": "Admin-only private notes",
          "type": "P",
          "members": ["sysadmin"]
        }
      ]
    },
    "marketing": {
      "name": "marketing",
      "displayName": "Marketing",
      "description": "Marketing department team",
      "type": "O",
      "channels": [
        {
          "name": "campaigns",
          "displayName": "Campaigns",
          "purpose": "Marketing campaign planning",
          "type": "O",
          "members": ["sysadmin"]
        }
      ]
    }
  }
}
```

### Configuration Options

#### Users

Each user in the `users` array has the following properties:

- `username` (required): The login name for the user
- `email` (required): The user's email address
- `password` (required): The user's password
- `nickname` (optional): A display name for the user
- `isSystemAdmin` (required): Whether the user should have system admin privileges
- `teams` (required): An array of team names the user should belong to

#### Teams

Each team in the `teams` map has the following properties:

- `name` (required): The team name (URL-friendly identifier)
- `displayName` (required): The human-readable team name
- `description` (optional): A description of the team
- `type` (optional): The team type - "O" for open (default), "I" for invite only
- `channels` (optional): An array of channel configurations for this team (see below)

#### Channels

Each channel in a team's `channels` array has the following properties:

- `name` (required): The channel name (URL-friendly identifier, lowercase with no spaces)
- `displayName` (required): The human-readable channel name
- `purpose` (optional): A brief description of the channel's purpose
- `header` (optional): Text that appears in the channel header
- `type` (optional): The channel type - "O" for public (default), "P" for private
- `members` (optional): An array of usernames to add to this channel

## Usage

### Building the Setup Tool

```bash
cd mattermost
go build -o mmsetup ./cmd
```

### Running

```bash
# Use default config.json in root directory
./mmsetup setup

# Specify a custom config file path
./mmsetup setup -config /path/to/config.json

# Display help for config.json format
./mmsetup help-config

# Show login information
./mmsetup echo-logins
```

### Running Development Commands

**Important**: Development commands should be run from the repository root directory:

```bash
# Run setup from repository root (NOT from mattermost subdirectory)
cd /path/to/demo-kit
go run main.go setup

# Force reinstall local plugins
go run main.go setup --reinstall-plugins local

# Force reinstall all plugins
go run main.go setup --reinstall-plugins all
```

### Plugin Management

The setup tool uses a JSONL-based plugin management system where plugins are defined in `bulk_import.jsonl`:

#### JSONL Plugin Configuration

Plugins are configured as JSON objects in the `bulk_import.jsonl` file:

```json
{"type": "plugin", "plugin": {"source": "github", "github_repo": "mattermost/mattermost-plugin-playbooks", "plugin_id": "playbooks", "name": "Playbooks", "force_install": false}}
{"type": "plugin", "plugin": {"source": "local", "path": "./apps/weather-plugin", "plugin_id": "com.coltoneshaw.weather", "name": "Weather Plugin", "force_install": false}}
```

#### Plugin Configuration Fields

- **`source`** (required): Either "github" or "local"
- **`github_repo`** (required for GitHub plugins): Repository in "owner/repo" format
- **`path`** (required for local plugins): Relative path to plugin directory
- **`plugin_id`** (required): Unique plugin identifier matching the plugin manifest
- **`name`** (required): Human-readable name for logging
- **`force_install`** (optional): Whether to force reinstall (default: false)

#### GitHub Plugins
- Must have GitHub releases with .tar.gz assets
- Downloads latest release automatically
- Plugin ID must match the actual plugin manifest ID

#### Local Plugins
- Must have a Makefile with `clean` and `dist` targets
- `make dist` must produce a .tar.gz file in the plugin's `dist/` directory
- Plugin path should be relative to the mmsetup binary location

#### Command Line Options

```bash
# Standard setup - skips already installed plugins
go run main.go setup

# Force reinstall local plugins only
go run main.go setup --reinstall-plugins local

# Force reinstall all plugins (local + GitHub)
go run main.go setup --reinstall-plugins all
```

**Note**: These commands should be run from the repository root directory, not from the mattermost subdirectory.

#### Adding New Plugins

1. Add a plugin entry to `bulk_import.jsonl`
2. For GitHub plugins: specify `github_repo` and `plugin_id`
3. For local plugins: specify `path` and ensure Makefile exists
4. Run setup command - plugins are processed automatically

#### Processing Order
1. GitHub plugins are processed first
2. Local plugins are processed second
3. Within each type, plugins are processed in JSONL file order

### Custom User Attributes

The setup tool supports custom user profile attributes that appear in user profiles and can be synchronized with LDAP when using the `--ldap` flag.

#### How to Add a Custom Attribute

To add a new custom attribute like "Department":

1. **Define the attribute** in `bulk_import.jsonl`:
```json
{"type": "user-attribute", "attribute": {"name": "department", "display_name": "Department", "type": "text", "required": false, "ldap": "departmentNumber", "visibility": "when_set"}}
```

2. **Assign values to users** in `bulk_import.jsonl`:
```json
{"type": "user-profile", "user": "john.smith", "attributes": {"department": "Security Forces", "rank": "Colonel"}}
{"type": "user-profile", "user": "maria.rodriguez", "attributes": {"department": "Operations", "rank": "Colonel"}}
```

3. **Run setup** to create the custom field:
```bash
./mmsetup setup
```

#### Attribute Configuration Options

- **`name`** (required): Internal identifier used in user-profile entries
- **`display_name`** (required): Label shown in the user interface
- **`type`** (required): Field type (`text`, `select`, etc.)
- **`required`** (optional): Whether all users must have this field
- **`ldap`** (optional): LDAP attribute to map to (e.g., `departmentNumber`, `employeeType`)
- **`visibility`** (optional): Who can see this field (`public`, `when_set`, `private`)
- **`hide_when_empty`** (optional): Hide field when no value is set

#### Assigning Attributes to Users

Use `user-profile` entries to assign attribute values:

```json
{"type": "user-profile", "user": "username", "attributes": {"attribute_name": "value"}}
```

- **`user`**: Must match an existing username
- **`attributes`**: Map of attribute names to values
- **Character limit**: All values must be 64 characters or less

#### LDAP Integration

When using `--ldap`, any attribute with an `ldap` field gets added as an LDAP attribute using the exact name you specify:

```json
{"type": "user-attribute", "attribute": {"name": "department", "display_name": "Department", "type": "text", "ldap": "departmentNumber"}}
{"type": "user-attribute", "attribute": {"name": "security_level", "display_name": "Security Level", "type": "text", "ldap": "securityClearance"}}
{"type": "user-attribute", "attribute": {"name": "rank", "display_name": "Rank", "type": "text", "ldap": "employeeType"}}
```

The system will:
1. Create LDAP attributes using exactly the names specified in the `ldap` field
2. Populate those LDAP attributes with values from user-profile entries  
3. Synchronize the data back to Mattermost user profiles

You can use any LDAP attribute name you want - both standard LDAP schema attributes (like `employeeType`, `departmentNumber`) or custom attributes (like `rank`, `securityLevel`).

#### Important Notes

- **Validation**: Setup will fail if any attribute value exceeds 64 characters
- **Matching**: Attribute names in user-profile entries must match those defined in user-attribute entries
- **LDAP Sync**: When using `--ldap`, attributes are automatically synchronized after user creation

### LDAP Groups

The setup tool supports creating and managing LDAP groups through the JSONL file format. Groups are automatically created in LDAP and linked to Mattermost when using the `--ldap` flag.

#### Group Configuration

Add groups to your `bulk_import.jsonl` file using the `user-groups` type:

```jsonl
{"type": "user-groups", "group": {"name": "command_staff", "id": "grp_002", "members": ["charles.armstrong", "maria.rodriguez", "john.smith"]}}
```

#### Group Configuration Fields

- `name` (required): The group name (will be used as the LDAP CN)
- `id` (required): A unique identifier for the group (stored as uniqueID LDAP attribute)
- `members` (required): Array of usernames who should be members of this group

#### LDAP Group Structure

Groups are created in LDAP with the following structure:

- **DN Format**: `cn={group_name},ou=groups,dc=planetexpress,dc=com`
- **Object Classes**: `groupOfNames`, `top`, and the custom auxiliary class for uniqueID
- **Attributes**:
  - `cn`: Group name
  - `uniqueID`: Unique group identifier
  - `member`: Array of user DNs in format `uid={username},ou=people,dc=planetexpress,dc=com`

#### Group Management Features

- **Smart Sync**: Groups are synchronized to match the JSONL configuration
  - Users not in the JSONL are removed from the group
  - Users in the JSONL but not in the group are added
  - No changes are made if membership already matches
- **Mattermost Integration**: Groups are automatically linked to Mattermost via the LinkLdapGroup API
- **Schema Extension**: Includes a custom `uniqueID` attribute for group identification

#### Example Groups

```jsonl
{"type": "user-groups", "group": {"name": "all_members", "id": "grp_001", "members": ["admin.user", "charles.armstrong", "maria.rodriguez"]}}
{"type": "user-groups", "group": {"name": "command_staff", "id": "grp_002", "members": ["charles.armstrong", "maria.rodriguez"]}}
{"type": "user-groups", "group": {"name": "operations_team", "id": "grp_003", "members": ["maria.rodriguez", "robert.williams"]}}
{"type": "user-groups", "group": {"name": "intelligence_team", "id": "grp_004", "members": ["james.thompson", "grace.turner"]}}
```

### Data Management

```bash
# Reset all teams and users (with confirmation prompt)
./mmsetup reset
```

## Features

### JSONL-Based Plugin Management
- **Configuration-Driven**: Plugins defined in `bulk_import.jsonl` for easy management
- **Automatic Plugin Detection**: Skips already installed plugins to avoid conflicts
- **Dual Source Support**: Handles both GitHub releases and local plugin development
- **Force Installation Options**: Granular control over plugin reinstallation
- **Smart Processing Order**: GitHub plugins first, then local plugins for optimal dependency handling
- **Build Integration**: Automatic building of local plugins with Makefile support

### Robust Data Import
- **Two-Phase Import**: Separates infrastructure (teams/channels) from user data for better reliability
- **Custom Type Support**: Handles channel categories and commands alongside standard Mattermost data
- **Error Recovery**: Continues processing even if individual items fail
- **File Lifecycle Management**: Proper cleanup of temporary files and uploads

### Safety and Validation
- **Confirmation Prompts**: Destructive operations require explicit confirmation with data counts
- **Idempotent Operations**: Safe to run setup multiple times without data corruption
- **Comprehensive Logging**: Clear feedback on what actions are being taken
- **License Validation**: Ensures Mattermost Enterprise features are available

## Applications

The demo kit includes several integrated applications:

- **Weather App**: Provides weather data via slash commands
- **Flight App**: Tracks flight departures
- **Main Mattermost Server**: Enterprise Edition deployment

## LDAP Troubleshooting

When using the `--ldap` flag, you may need to troubleshoot LDAP connectivity and user data. Here are useful commands for diagnosing LDAP issues:

### Testing LDAP Connection

```bash
# Test basic LDAP connectivity
docker exec openldap ldapsearch -x -H ldap://localhost:10389 \
  -D "cn=admin,dc=planetexpress,dc=com" \
  -w GoodNewsEveryone \
  -b "dc=planetexpress,dc=com" \
  "(objectClass=*)"
```

### Searching for Specific Users

```bash
# Search for a specific user by username
docker exec openldap ldapsearch -x -H ldap://localhost:10389 \
  -D "cn=admin,dc=planetexpress,dc=com" \
  -w GoodNewsEveryone \
  -b "dc=planetexpress,dc=com" \
  "(uid=charles.armstrong)"

# Search for all users
docker exec openldap ldapsearch -x -H ldap://localhost:10389 \
  -D "cn=admin,dc=planetexpress,dc=com" \
  -w GoodNewsEveryone \
  -b "dc=planetexpress,dc=com" \
  "(objectClass=Person)"
```

### Connection Troubleshooting

If you encounter "Can't contact LDAP server" errors with the standard approach:

```bash
# This may fail with connection errors
docker exec -it openldap ldapsearch -x \
  -D "cn=admin,dc=planetexpress,dc=com" \
  -w GoodNewsEveryone \
  -b "dc=planetexpress,dc=com" \
  "(uid=username)"
```

Use the alternative localhost connection method:

```bash
# Alternative: Use localhost inside container
docker exec openldap ldapsearch -x -H ldap://localhost:10389 \
  -D "cn=admin,dc=planetexpress,dc=com" \
  -w GoodNewsEveryone \
  -b "dc=planetexpress,dc=com" \
  "(uid=username)"
```

### Common Issues

- **Connection refused**: Ensure the OpenLDAP container is running and healthy
- **Attribute type undefined**: Check LDAP schema and attribute mappings in the code
- **Authentication failed**: Verify bind DN and password in environment variables

# JSONL Bulk Import Generation Instructions for LLMs

I want you to generate a jsonl file following the below parameters. Adhere firmly to the guide below and follow the use case. THINK HARD.

## Overview

This document provides comprehensive instructions for generating a Mattermost bulk import JSONL file. The file should contain realistic organizational data with proper relationships, hierarchies, and communication patterns. **All content, including posts, will be imported through Mattermost's standard bulk import process.**

## Use Case Adaptation

The below information is to be used for the use case of this example environment. 

**Military Base** → **Corporate Office**:
- Ranks → Job Titles (Manager, Director, VP)
- Security Clearances → Access Levels (Standard, Confidential, Executive)
- Mission Channels → Project Channels
- Command Structure → Management Hierarchy
- Military Protocol → Corporate Communication Standards

**Key Adaptation Points:**
1. Terminology and language style
2. Channel purposes and naming conventions
3. User attribute types and values
4. Communication hierarchy and protocols
5. Group structures and membership rules

**Total Posts**: 1000
**Total Users:**: 50
**Total Channels**: 25

### Post Distribution

- **70% Regular Posts**: Standard communication, updates, questions
- **20% Posts with Replies**: Conversations with 5-6 relevant responses
- **10% Call Posts**: Coordination calls, briefings, meetings

## Core Parameters (Always Honor These)

When generating a bulk import file, you MUST respect these user-specified parameters:
- **Use Case Description**: The organizational context (e.g., "USAF military base", "corporate tech company", "healthcare system")
- **Total Posts**: Exact number of posts to generate
- **Total Channels**: Exact number of channels to create
- **Total Users**: Exact number of users to create

## Import Types and Processing Order

The system processes import types in this order:
1. **Standard Mattermost Types** (processed by bulk import): `version`, `team`, `channel`, `user`, `post`
2. **Custom Types** (processed by setup tool): `user-attribute`, `user-profile`, `channel-category`, `channel-banner`, `command`, `plugin`

## Import Types and Structure

THE FILE MUST INCLUDE EACH TYPE BELOW BASED ON THE INSTRUCTIONS.

### 1. Version Declaration

**Always start the file with:**
```json
{"type": "version", "version": 1}
```

### 2. User Attributes
Define custom profile fields that will appear on user profiles:
```json
{"type": "user-attribute", "attribute": {
  "name": "field_name",
  "display_name": "Display Name", 
  "type": "text",
  "hide_when_empty": false,
  "required": true,
  "ldap": "ldap_attribute_name",
  "saml": "",
  "options": null,
  "sort_order": 0,
  "value_type": "",
  "visibility": "when_set"
}}
```

These attributes can be anything relevant to the use case in question. Some common ones to consider are Rank/Title, Department/Unit, Location, Security Clearance, Manager/Supervisor, and Bio/Specialization. 

**Attributes must be 64 characters or shorter**

### 3. Plugins

Install necessary plugins before creating content:
```json
{"type": "plugin", "plugin": {
  "source": "github",
  "github_repo": "mattermost/mattermost-plugin-playbooks",
  "plugin_id": "playbooks",
  "name": "Playbooks",
  "force_install": false
}}
```

These plugins must exist on Github for Mattermost. 

The concept of local plugins also exists. These can be found in the `demo-kit/apps` directory with descriptions of how to use it and what they do. 

**required plugins:**
- `mattermost/mattermost-plugin-playbooks` (workflow management)
- `mattermost/mattermost-plugin-ai` (AI assistance)

### 4. Team Creation

Create the main organizational team:
```json
{"type": "team", "team": {
  "name": "team-name",
  "display_name": "Team Display Name",
  "type": "O",
  "description": "Team description"
}}
```

Primarily generate with one team, unless there is a specific use case for multiple teams.

### 5. Channel Creation
Create channels with proper hierarchy and purpose:
```json
{"type": "channel", "channel": {
  "team": "team-name",
  "name": "channel-name",
  "display_name": "Channel Display Name", 
  "type": "O",
  "purpose": "Brief channel purpose",
  "header": "**Channel header** with important info and [links](https://example.com)"
}}
```

**Channel Types:**
- `"O"` = Public channel
- `"P"` = Private channel

**Channel Organization Tips:**
- Group by department/function
- Include both operational and social channels
- Use descriptive purposes and headers with relevant links/instructions

### 6. Channel Categories

Organize channels into logical categories:
```json
{"type": "channel-category", "category": "Category Name", "team": "team-name", "channels": ["channel1", "channel2", "channel3"]}
```

A single category can hold multiple channels. The categories must be relevant to the channels and make sense in the use case. 

### 7. Channel Banners

Add important banners for critical channels:
```json
{"type": "channel-banner", "banner": {
  "team": "team-name",
  "channel": "channel-name", 
  "text": "IMPORTANT BANNER TEXT",
  "background_color": "#FF0000",
  "enabled": true
}}
```

Banners should be used within the environment, but sparringly. They should be used to convey importance of a channel, classification of a channel, or other priority information.

**Suggested Banner Colors:**
- `#FF0000` = Red (urgent/critical)
- `#FF8C00` = Orange (important)
- `#FFD700` = Yellow (warning)
- `#0066CC` = Blue (information)


### 8. Users
Create users with proper attributes and team membership:
```json
{"type": "user", "user": {
  "username": "john.smith", //unique
  "email": "john.smith@organization.com", //unique
  "password": "password", // SHOULD ALWAYS BE password
  "nickname": "John Smith",
  "first_name": "John",
  "last_name": "Smith", 
  "position": "Manager",
  "roles": "system_user",
  "teams": [{
    "name": "team-name",
    "roles": "team_user",
    "channels": [
      {"name": "channel1", "roles": "channel_user"},
      {"name": "channel2", "roles": "channel_user"}
    ]
  }]
}}
```

**Important:** Each user must specify which channels they have access to within the teams array.

### 9. User Profiles

These profile values must exist in the user attributes section above. They must be relevant to the use case. Below is an example structure.

Assign custom attribute values to users:
```json
{"type": "user-profile", "user": "john.smith", "attributes": {
  "rank": "Manager",
  "department": "Operations", 
  "location": "Building A",
  "clearance": "Level 2",
  "bio": "Experienced operations manager",
  "supervisor": "jane.doe"
}}
```

### 10. User Groups

Create @mentionable groups for easy communication:
```json
{"type": "user-groups", "group": {
  "name": "group_name",
  "id": "grp_001", 
  "allow_reference": true,
  "members": ["user1", "user2", "user3"]
}}
```

User groups should be relevant to the use case and contain all or partial users. Grouping users based on their role into a specific group is ideal. The groupings of users should make sense and fit within the use case.

**Group Examples:**
- `all_members` - Everyone
- `management_team` - Managers and above
- `department_heads` - Department leaders
- `on_call_team` - Emergency response team

### 11. Posts
Generate realistic conversation posts that will be imported with users:

#### Regular Posts:
```json
{"type": "post", "post": {
  "team": "team-name",
  "channel": "channel-name",
  "user": "username",
  "message": "Post content with proper context",
  "create_at": 1734531900000,
  "replies": [
    {
      "user": "responding_user",
      "message": "Reply content", 
      "create_at": 1734531920000
    }
  ]
}}
```

#### Call Posts:
```json
{"type": "post", "post": {
  "team": "team-name", 
  "channel": "channel-name",
  "user": "username",
  "message": "Call ended",
  "type": "custom_calls",
  "create_at": 1734531900000,
  "props": {
    "title": "Call Title",
    "end_at": 1734531930000,
    "start_at": 1734531900000,
    "attachments": [{
      "id": 0,
      "ts": null,
      "text": "Call ended",
      "color": "",
      "title": "Call ended", 
      "fields": null,
      "footer": "",
      "pretext": "",
      "fallback": "Call ended",
      "image_url": "",
      "thumb_url": "",
      "title_link": "",
      "author_icon": "",
      "author_link": "", 
      "author_name": "",
      "footer_icon": ""
    }],
    "from_plugin": "true",
    "participants": null
  }
}}
```

**Call Post Rules:**
- Always use `"message": "Call ended"`
- Include `"type": "custom_calls"` in the post object
- Use proper props structure with attachments array
- `from_plugin` must be string `"true"`, not boolean

### 12. Commands
Commands should exist in the plugins that are being imported. You can read the local plugin documentation for more information or the plugin information on github that you're fetching from. 

Execute slash commands to set up integrations:
```json
{"type": "command", "command": {
  "team": "team-name",
  "channel": "channel-name", 
  "text": "/weather --location \"New York\" --subscribe --frequency 120m"
}}
```

## Content Generation Guidelines

### Realistic Communication Patterns

1. **Hierarchy Respect**: Lower-ranking members should communicate respectfully to higher-ranking members
2. **Domain Expertise**: Technical specialists should provide relevant expertise in their channels
3. **Operational Urgency**: Emergency/critical channels should have appropriate urgency in communication
4. **Cross-Channel References**: Use @mentions and channel references to create realistic workflow

### Timestamp Strategy

Posts are imported through the bulk import system with their specified timestamps:
- Use timestamps that span a realistic timeframe (last week to last few minutes)
- Most recent posts should have highest timestamps
- Ensure replies have timestamps after their parent posts
- Call posts should have realistic duration (start_at to end_at typically 5-30 minutes)

### User Channel Membership Rules

1. **Leadership**: Access to most/all channels including private strategic channels
2. **Department Staff**: Access to their department channels + relevant cross-functional channels  
3. **Specialists**: Access to their specialty channels + general announcement channels
4. **All Users**: Should have access to general announcement and social channels

### Reply Generation

When adding replies to posts:
- Ensure replying users have access to that channel
- Maintain hierarchy (juniors acknowledge seniors appropriately)
- Keep replies contextually relevant to the original post
- Use realistic timing (replies within minutes to hours of original post)

## User Group Relationships

Groups should reflect organizational structure:
- **Functional Groups**: `@engineering_team`, `@sales_team`, `@medical_staff`
- **Hierarchical Groups**: `@management`, `@supervisors`, `@executives`  
- **Operational Groups**: `@on_call`, `@incident_response`, `@security_team`
- **Communication Groups**: `@all_staff`, `@department_heads`, `@project_leads`

Users can belong to multiple groups based on their roles and responsibilities.

## Technical Notes

- All timestamps should be in milliseconds since Unix epoch
- Channel and user names should be lowercase with no spaces
- Display names can have proper capitalization and spaces
- Email addresses should follow realistic organizational patterns
- Passwords should meet basic security requirements
- Custom types are processed by the setup tool after bulk import completes

## Plugin Slash Commands

The demo kit includes three subscription-based plugins that provide real-time data updates through slash commands:

### Weather Plugin
**Purpose**: Provides mock weather information and monitoring for operational planning and safety decisions.

- `/weather subscribe --location Tokyo --frequency 1h` - Subscribe to weather updates every hour
- `/weather subscribe --location "New York" --frequency 30m` - Subscribe with 30-minute frequency


### FlightAware Plugin  
**Purpose**: Tracks flight operations and provides real-time flight status updates for operational coordination.

- `/flights subscribe --airport LAX --frequency 1800` - Subscribe to updates every 30 minutes


### Mission Operations Plugin
**Purpose**: Coordinates mission planning, status tracking, and operational communications.

- `/mission start --name {name} --callsign {callsign} --departureAirport {airport-code} --arrivalAirport LAX --crew @{username} @{username}` - Create a new mission
- `/mission status completed --id mission_123` - Update status with mission ID
- `/mission subscribe --type stalled,in-air --frequency 3600` - Subscribe to specific status updates
- `/mission subscribe --type all --frequency 1800` - Subscribe to all mission status updates

