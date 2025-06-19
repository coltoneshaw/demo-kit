Evacuation Operations During Conflict: A Mattermost-Enabled Response
--------------------------------------------------------------------

In high-threat environments like embassy evacuations, coordination across U.S. agencies becomes critical amid chaos. Below is a scenario illustrating the urgency, followed by Mattermost’s role in streamlining operations.

**Scenario: Evacuation Under Fire**
-----------------------------------

*Kabul, 2025*  
Rocket impacts near the embassy shatter windows as crowds surge against blast walls. Diplomatic Security (DS) agents scramble to secure sensitive materials while Marines form perimeter defenses. Civilians frantically seek evacuation passes as intelligence officers relay real-time threat updates: insurgents control key highways. State Department officials coordinate airlift slots via encrypted channels, but fragmented communication between DoD, CIA, and host-nation forces causes delays. A bus convoy to the airport stalls—armed groups block the route. With cell networks jammed and lives at stake, disjointed agency silos threaten the mission.

**Agencies and Components Involved**
------------------------------------

*   **Civilian Leadership**: State Department (evacuation authority), USAID (civilian support)
    
*   **Defense**: U.S. Marines (security), SOCOM (special ops), NORTHCOM (logistics)
    
*   **Intelligence**: CIA (on-ground intel), NSA (signals), DIA (threat analysis)
    
*   **Host Nation**: Military/police forces (route security, intel sharing)
    

**Mattermost’s Role in Unifying Operations**
--------------------------------------------

Mattermost transforms chaotic multi-agency coordination through **shared channels** and **cross-platform integration**, enabling real-time collaboration even in contested environments.

**1\. Unified Agency Coordination via Shared Channels**
-------------------------------------------------------

*   **Cross-Domain Collaboration**:  
    Shared channels connect State Department, DoD, and intelligence units in a single workspace. For example:
    
    *   `#evac-airlift`: State Department updates flight manifests while Marines confirm perimeter security.
        
    *   `#intel-feed`: CIA and NSA fuse satellite imagery and HUMINT into actionable alerts.
        
    *   **Zero Trust Security**: Role-Based Access Control (RBAC) limits channel visibility (e.g., civilians see evacuation routes; SOCOM sees hostile force locations).
        

**2\. Host-Nation Integration via Matterbridge**
------------------------------------------------

*   **Interoperability with Local Forces**:  
    Matterbridge links Mattermost to host-nation systems (XMPP, IRC, Jabber):
    
    *   Host-nation police report checkpoint status via IRC, auto-routed to `#route-security`.
        
    *   Embassy staff coordinate bus movements with local military using XMPP, mirrored in `#convoy-ops`.
        
    *   **DDIL Resilience**: Queued messages sync once connectivity resumes, ensuring no data loss during network outages.
        

**3\. Automated Workflow Orchestration**
----------------------------------------

*   **Evacuation Playbooks**:  
    Pre-built playbooks automate critical workflows:
    
    *   *Personnel Accountability*: Civilians check in via mobile app; automated manifests update flight queues.
        
    *   *Threat Response*: AI-driven alerts trigger reroutes when Playbooks detect highway blockages via intel feeds.
        
    *   *Resource Allocation*: Dynamic dashboards track buses, medevac units, and airlift capacity.
        

**4\. Cross-Platform Accessibility**
------------------------------------

*   **Mobile/Desktop Sync**:  
    Marines access channel updates on tactical tablets; diplomats coordinate via CUI-enabled phones—all with end-to-end encryption.
    
*   **Multilingual Support**:  
    Real-time translation APIs convert host-nation reports into English, reducing miscommunication.
    

**Outcome: Cohesive Response Amid Chaos**
-----------------------------------------

In our scenario, Mattermost’s shared channels merge embassy, military, and host-nation comms into a single operational view. Playbooks reroute convoys around blockages, while Matterbridge-integrated host-nation alerts prevent ambushes. Real-time dashboards display asset locations, accelerating decisions as airlifts proceed under fire. This unified approach turns fragmented efforts into a synchronized evacuation—proving critical when minutes determine survival.



# JSONL Bulk Import Generation Instructions for LLMs



## Overview

This document provides comprehensive instructions for generating a Mattermost bulk import JSONL file. The file should contain realistic organizational data with proper relationships, hierarchies, and communication patterns. **All content, including posts, will be imported through Mattermost's standard bulk import process.**

## Use Case Adaptation


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

