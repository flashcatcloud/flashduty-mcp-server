# Flashduty MCP Server Package

This package provides Model Context Protocol (MCP) tools for interacting with the Flashduty API, based on the official OpenAPI specification.

## Architecture

The implementation follows the same patterns as the GitHub MCP Server:

- **Client (`client.go`)**: HTTP client for Flashduty API with app_key authentication
- **Server helpers (`server.go`)**: Common parameter handling and response utilities
- **Tools (`tools.go`)**: Tool set organization and registration
- **Members (`members.go`)**: Member management tools implementation

## Authentication

Uses Flashduty's app_key authentication method via query parameters:
```
https://api.flashcat.cloud/member/list?app_key=YOUR_APP_KEY
```

## Members Toolset

### Available Tools

#### 1. `flashduty_get_member_list`
- **Method**: POST `/member/list`
- **Description**: Get paginated list of members
- **Parameters**:
  - `p` (number): Page number, starting from 1 (default: 1)
  - `limit` (number): Items per page (default: 20) 
  - `query` (string): Search keyword for member name
  - `role_id` (number): Filter by role ID
  - `orderby` (string): Sort field (created_at, updated_at)
  - `asc` (boolean): Sort ascending (default: false)

#### 2. `flashduty_invite_member`
- **Method**: POST `/member/invite`
- **Description**: Invite a new member to the account
- **Parameters**:
  - `email` (string): Email address (required if phone not provided)
  - `phone` (string): Phone number (required if email not provided)
  - `country_code` (string): Country code (default: CN)
  - `member_name` (string): Member name (required when phone provided)
  - `ref_id` (string): Reference ID
  - `role_ids` (string): Comma-separated list of role IDs

#### 3. `flashduty_update_member_info`
- **Method**: POST `/member/info/reset`
- **Description**: Update member information
- **Identity Parameters** (exactly one required):
  - `member_id` (number): Member ID
  - `member_name` (string): Member name
  - `phone` (string): Member phone
  - `email` (string): Member email
  - `ref_id` (string): Reference ID
- **Update Parameters** (at least one required):
  - `new_phone` (string): New phone number
  - `new_country_code` (string): New country code
  - `new_email` (string): New email address
  - `new_member_name` (string): New member name
  - `new_time_zone` (string): New timezone (tzdata format)
  - `new_locale` (string): New locale (zh-CN, en-US)
  - `new_ref_id` (string): New reference ID

#### 4. `flashduty_delete_member`
- **Method**: POST `/member/delete`
- **Description**: Delete a member from the account
- **Parameters** (exactly one required):
  - `member_id` (number): Member ID
  - `phone` (string): Member phone
  - `email` (string): Member email
  - `ref_id` (string): Reference ID

## Key Implementation Details

### API Differences from REST Standards
- All operations use POST method, not GET/PUT/DELETE
- Uses specific Flashduty API paths (`/member/list`, `/member/invite`, etc.)
- Request body structure follows Flashduty's schema exactly
- Authentication via query parameter `app_key` instead of Bearer token

### Request/Response Structure
```go
// Member list request
{
  "p": 1,
  "limit": 20,
  "query": "search term"
}

// Member invite request  
{
  "members": [
    {
      "email": "user@example.com",
      "member_name": "User Name",
      "country_code": "CN",
      "role_ids": [1, 2]
    }
  ]
}

// Member update request
{
  "member_id": 123,
  "updates": {
    "member_name": "New Name",
    "email": "new@example.com"
  }
}
```

### Error Handling
Flashduty API returns errors in this format:
```go
{
  "error": {
    "code": "InvalidParameter",
    "message": "Error description"
  }
}
```

## Testing

The package includes comprehensive unit tests using mock clients:

```bash
cd pkg/flashduty
go test -v
```

Tests cover:
- Tool metadata validation
- Request parameter handling
- API request construction
- Response parsing
- Error scenarios

## Future Expansion

The current implementation focuses on member management. The architecture supports easy addition of other Flashduty toolsets:

- Teams management (`/team/*`)
- Schedules management (`/schedule/*`)
- Channels management (`/channel/*`)
- Incidents management (`/incident/*`)
- Alerts management (`/alert/*`)
- Analytics (`/analytics/*`)

Each toolset would follow the same pattern established in the members implementation. 