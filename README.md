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

