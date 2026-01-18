# External API (EAPI) Documentation

## Overview

The External API (EAPI) provides secure, token-based access to Donetick features for external applications and integrations. It's designed for long-lived access tokens and offers a subset of functionality optimized for mobile apps and third-party integrations.

## Base URL

```
https://your-donetick-instance.com/eapi/v1
```

## Authentication

All EAPI endpoints require authentication using an API token.

### Header Format

```http
secretkey: YOUR_API_TOKEN_HERE
```

**Important:** Use the header name `secretkey`, NOT `Authorization: Bearer`

### Getting an API Token

1. Log in to Donetick web UI
2. Go to Settings → API Tokens
3. Generate a new token
4. Store it securely (tokens are only shown once)

### Authentication Errors

```json
// Missing token
{"error": "API token required"}

// Invalid token
{"error": "Invalid API token"}

// Disabled user
{"error": "User account is disabled"}
```

---

## Response Format

### Empty Arrays

All endpoints that return arrays will return an **empty array `[]`** instead of `null` when there are no items. This ensures consistent data handling in client applications.

**Example:**
```json
// When there are no chores:
[]

// NOT:
null
```

### Wrapped Responses

Some endpoints wrap the response in a `res` field:
```json
{
  "res": [...],
  "count": 10,
  "hasMore": true
}
```

---

## Endpoints

### Things

#### GET /eapi/v1/things/

Get all things for the authenticated user.

**Request:**
```bash
curl -H "secretkey: YOUR_TOKEN" \
  https://donetick.example.com/eapi/v1/things/
```

**Response:**
```json
[
  {
    "id": 1,
    "userID": 1,
    "circleId": 0,
    "name": "Myčka prázdná",
    "state": "true",
    "type": "boolean",
    "thingChores": null,
    "updatedAt": "2025-12-28T21:20:14.940453686Z",
    "createdAt": "2025-12-28T13:40:33.962724485+01:00"
  }
]
```

**Note:** Both `/eapi/v1/things` and `/eapi/v1/things/` work (trailing slash optional)

---

#### GET /eapi/v1/things/:id

Get a specific thing by ID, including associated chores.

**Request:**
```bash
curl -H "secretkey: YOUR_TOKEN" \
  https://donetick.example.com/eapi/v1/things/1
```

**Response:**
```json
{
  "thing": {
    "id": 1,
    "userID": 1,
    "circleId": 0,
    "name": "Myčka prázdná",
    "state": "true",
    "type": "boolean",
    "thingChores": [
      {
        "thingId": 1,
        "choreId": 2,
        "triggerState": "false",
        "condition": ""
      },
      {
        "thingId": 1,
        "choreId": 15,
        "triggerState": "true",
        "condition": ""
      }
    ],
    "updatedAt": "2025-12-28T21:20:14.940453686Z",
    "createdAt": "2025-12-28T13:40:33.962724485+01:00"
  }
}
```

**thingChores explained:**
- `triggerState`: When thing reaches this state, the chore is triggered/scheduled
- `choreId`: ID of the chore that gets scheduled
- `condition`: Additional condition logic (optional)

---

#### GET /eapi/v1/things/:id/state

Update thing state to a specific value.

**Parameters:**
- `state` (required): New state value

**Request:**
```bash
# For boolean thing
curl -H "secretkey: YOUR_TOKEN" \
  "https://donetick.example.com/eapi/v1/things/1/state?state=false"

# For numeric thing
curl -H "secretkey: YOUR_TOKEN" \
  "https://donetick.example.com/eapi/v1/things/2/state?state=5"

# For text thing
curl -H "secretkey: YOUR_TOKEN" \
  "https://donetick.example.com/eapi/v1/things/3/state?state=active"
```

**Response:**
```json
{}
```

**Side Effects:**
- Updates thing state in database
- Evaluates triggers and schedules chores if trigger conditions are met

**Errors:**
```json
{"error": "Invalid state value"}        // Missing state parameter
{"error": "Invalid state for thing"}    // State doesn't match thing type
{"error": "Thing not found"}            // Invalid thing ID
```

---

#### GET /eapi/v1/things/:id/state/change

Change thing state with operations (for numeric things) or set value.

**Parameters:**
- `op` (optional): Numeric value to add/subtract (e.g., `+5`, `-3`, `1`)
- `set` (optional): Set to specific value directly

**Request:**
```bash
# Increment by 1
curl -H "secretkey: YOUR_TOKEN" \
  "https://donetick.example.com/eapi/v1/things/2/state/change?op=1"

# Decrement by 2
curl -H "secretkey: YOUR_TOKEN" \
  "https://donetick.example.com/eapi/v1/things/2/state/change?op=-2"

# Set to specific value
curl -H "secretkey: YOUR_TOKEN" \
  "https://donetick.example.com/eapi/v1/things/1/state/change?set=false"
```

**Response:**
```json
{}
```

**Behavior:**
- `op`: Adds/subtracts from current numeric state
- `set`: Sets state to exact value (any type)
- Evaluates triggers after state change

**Errors:**
```json
{"error": "Invalid increment value"}    // Missing both op and set, or invalid op format
{"error": "Invalid state for thing"}    // Current state is not numeric (when using op)
```

---

### Chores

#### GET /eapi/v1/chore

Get all chores for the authenticated user's circle.

**Request:**
```bash
curl -H "secretkey: YOUR_TOKEN" \
  https://donetick.example.com/eapi/v1/chore
```

**Response:**
```json
{
  "res": [
    {
      "id": 1,
      "name": "Take out trash",
      "assignedTo": 5,
      "dueDate": "2026-01-03T10:00:00Z",
      "frequencyType": "daily",
      "points": 2,
      "isActive": true,
      "createdBy": 1,
      "updatedAt": "2026-01-02T15:30:00Z"
    }
  ]
}
```

---

#### GET /eapi/v1/chore/:id

Get a specific chore by ID.

**Request:**
```bash
curl -H "secretkey: YOUR_TOKEN" \
  https://donetick.example.com/eapi/v1/chore/15
```

**Response:**
```json
{
  "id": 15,
  "name": "Take out trash",
  "assignedTo": 5,
  "dueDate": "2026-01-03T10:00:00Z",
  "frequencyType": "daily",
  "points": 2,
  "isActive": true,
  "createdBy": 1,
  "updatedAt": "2026-01-02T15:30:00Z"
}
```

**Errors:**
```json
{"error": "Invalid chore ID"}    // Invalid ID format
{"error": "Chore not found"}     // Chore doesn't exist or access denied
```

---

#### GET /eapi/v1/chore/archived

Get archived chores for the authenticated user's circle.

**Request:**
```bash
curl -H "secretkey: YOUR_TOKEN" \
  https://donetick.example.com/eapi/v1/chore/archived
```

**Response:**
```json
{
  "res": [
    {
      "id": 42,
      "name": "Old recurring task",
      "assignedTo": 1,
      "dueDate": null,
      "frequencyType": "weekly",
      "points": 3,
      "isActive": false,
      "createdBy": 1,
      "archivedAt": "2025-12-15T10:00:00Z",
      "updatedAt": "2025-12-15T10:00:00Z"
    }
  ]
}
```

**Errors:**
```json
{"error": "Error fetching archived chores"}    // Database or access error
```

---

#### GET /eapi/v1/chore/history

Get chore completion history.

**Query Parameters:**
- `limit` (optional): Number of results (default: 10)
- `offset` (optional): Pagination offset (default: 0)

**Request:**
```bash
curl -H "secretkey: YOUR_TOKEN" \
  "https://donetick.example.com/eapi/v1/chore/history?limit=5"
```

**Response:**
```json
{
  "count": 10,
  "hasMore": false,
  "res": [
    {
      "id": 65,
      "choreId": 73,
      "performedAt": "2025-12-29T11:48:02.839860453Z",
      "completedBy": 1,
      "assignedTo": 1,
      "notes": null,
      "dueDate": "2025-12-29T11:50:00Z",
      "updatedAt": "2025-12-29T12:48:02.843655125+01:00",
      "createdAt": "2025-12-29T12:48:02.843655125+01:00",
      "status": 1,
      "points": 5
    }
  ]
}
```

---

#### GET /eapi/v1/chore/points

Get monthly points leaderboard for circle members.

**Request:**
```bash
curl -H "secretkey: YOUR_TOKEN" \
  https://donetick.example.com/eapi/v1/chore/points
```

**Response:**
```json
{
  "period": "month",
  "res": [
    {
      "userId": 10,
      "userName": "Bára",
      "points": 20,
      "rank": 1
    },
    {
      "userId": 1,
      "userName": "Tatinek",
      "points": 18,
      "rank": 2
    },
    {
      "userId": 8,
      "userName": "Kačenka",
      "points": 16,
      "rank": 3
    }
  ]
}
```

---

#### POST /eapi/v1/chore (Plus Members Only)

Create a new chore.

**Request:**
```bash
curl -X POST -H "secretkey: YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "New chore",
    "assignedTo": 1,
    "frequencyType": "daily",
    "points": 3
  }' \
  https://donetick.example.com/eapi/v1/chore
```

---

#### POST /eapi/v1/chore/:id/complete (Plus Members Only)

Mark a chore as complete.

**Request Body (optional):**
```json
{
  "completedBy": 7  // Optional: specify which user completed the chore
}
```

**Request:**
```bash
# Simple completion (attributed to API token owner)
curl -X POST -H "secretkey: YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}' \
  https://donetick.example.com/eapi/v1/chore/73/complete

# Specify who completed it (e.g., for multi-user apps)
curl -X POST -H "secretkey: YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"completedBy": 7}' \
  https://donetick.example.com/eapi/v1/chore/73/complete
```

**Response:**
```json
{
  "id": 73,
  "completedAt": "2026-01-02T18:33:42Z",
  "nextDueDate": "2026-01-03T18:00:00Z"
}
```

**Errors:**
```json
{"error": "Error getting chore"}           // Chore not found or access denied
{"error": "Chore is out of completion window"} // Too early to complete
{"error": "Error completing chore"}        // Database or logic error
```

**Side Effects:**
- Creates chore history entry
- Adds points to user
- Calculates next due date
- Rotates assignee (if configured)
- Executes Thing action (if configured)
- Triggers scheduled chores via Thing state changes

---

#### POST /eapi/v1/chore/:id/undo (Plus Members Only)

Undo last completion of a chore. Removes the completion from history, deducts points, and restores the chore to its previous state (due date, assignee).

**Request Body (optional):**
```json
{
  "newDueDate": "2026-01-20T10:00:00Z"  // Optional: override restored due date
}
```

**Request:**
```bash
curl -X POST -H "secretkey: YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"newDueDate": "2026-01-20T10:00:00Z"}' \
  https://donetick.example.com/eapi/v1/chore/123/undo
```

**Response:**
```json
{
  "message": "Completion undone successfully",
  "chore": {
    "id": 123,
    "name": "Clean kitchen",
    "nextDueDate": "2026-01-20T10:00:00Z",
    "assignedTo": 5,
    "status": 0,
    "isActive": true
  }
}
```

**Permissions:**
- User who completed the chore
- User assigned to the chore
- Admin or manager role

**Errors:**
```json
{"error": "No completion to undo"}
{"error": "Not authorized to undo this completion"}
```

**Side Effects:**
- Removes chore history entry
- Deducts points from user who completed it
- Restores previous due date and assignee

---

#### POST /eapi/v1/chore/:id/reject (Plus Members Only)

Reject a chore that is pending approval. Changes status from PENDING_APPROVAL to REJECTED. Does NOT deduct points or change due date.

**Request Body (optional):**
```json
{
  "note": "Reason for rejection"  // Optional rejection note
}
```

**Request:**
```bash
curl -X POST -H "secretkey: YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"note": "Needs more work"}' \
  https://donetick.example.com/eapi/v1/chore/123/reject
```

**Response:**
```json
{
  "id": 123,
  "name": "Clean kitchen",
  "status": 0,
  "nextDueDate": "2026-01-20T10:00:00Z",
  "assignedTo": 5
}
```

**Permissions:**
- Admin or manager role only

**Errors:**
```json
{"error": "Chore is not pending approval"}
{"error": "Only admins can reject chores"}
```

**Side Effects:**
- Updates chore status back to "no status"
- Chore remains scheduled at the same due date
- Assignee can complete it again

---

#### POST /eapi/v1/chore/:id/not-needed (Plus Members Only)

Mark a chore as "not needed". For one-time chores, archives them. For recurring chores, reschedules to next occurrence. No points awarded. Creates history entry with status NOT_NEEDED (7).

**Request Body (optional):**
```json
{
  "note": "Reason why not needed"  // Optional note
}
```

**Request:**
```bash
curl -X POST -H "secretkey: YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"note": "Dishwasher was empty"}' \
  https://donetick.example.com/eapi/v1/chore/123/not-needed
```

**Response:**
```json
// For one-time chore (archived):
{
  "id": 123,
  "name": "Clean kitchen",
  "isActive": false,
  "status": 0
}

// For recurring chore (rescheduled):
{
  "id": 123,
  "name": "Clean kitchen",
  "nextDueDate": "2026-01-21T10:00:00Z",
  "assignedTo": 6,
  "status": 0
}
```

**Permissions:**
- Any circle member can mark chore as not needed

**Side Effects:**
- One-time chores: archived (isActive = false)
- Recurring chores: rescheduled to next occurrence
- No points awarded
- Creates history entry with NOT_NEEDED status
- Rotates assignee for recurring chores

---

#### PUT /eapi/v1/chore/:id (Plus Members Only)

Update an existing chore.

**Request:**
```bash
curl -X PUT -H "secretkey: YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Updated chore name",
    "points": 5
  }' \
  https://donetick.example.com/eapi/v1/chore/73
```

---

#### DELETE /eapi/v1/chore/:id

Delete a chore.

**Request:**
```bash
curl -X DELETE -H "secretkey: YOUR_TOKEN" \
  https://donetick.example.com/eapi/v1/chore/73
```

---

### Circle

#### GET /eapi/v1/circle/members (Plus Members Only)

Get all members in the user's circle.

**Request:**
```bash
curl -H "secretkey: YOUR_TOKEN" \
  https://donetick.example.com/eapi/v1/circle/members
```

**Response:**
```json
[
  {
    "id": 1,
    "userId": 1,
    "circleId": 1,
    "role": "admin",
    "isActive": true,
    "createdAt": "2025-09-14T17:36:32.61447698Z",
    "updatedAt": "2025-12-29T12:48:02.84306697+01:00",
    "points": 33,
    "pointsRedeemed": 25,
    "username": "tomcer",
    "displayName": "Tatinek",
    "image": ""
  },
  {
    "id": 14,
    "userId": 7,
    "circleId": 1,
    "role": "member",
    "isActive": true,
    "points": 10,
    "pointsRedeemed": 0,
    "username": "tomcer_tomik",
    "displayName": "Tomík",
    "image": ""
  }
]
```

---

## Access Control

### Standard Endpoints
Available to all users with valid API token:
- GET /eapi/v1/things/*
- GET /eapi/v1/chore (list)
- GET /eapi/v1/chore/:id (detail)
- GET /eapi/v1/chore/archived
- GET /eapi/v1/chore/history
- GET /eapi/v1/chore/points
- POST /eapi/v1/chore (create)
- DELETE /eapi/v1/chore/:id

### Plus Member Only Endpoints
Require Plus membership:
- POST /eapi/v1/chore/:id/complete
- POST /eapi/v1/chore/:id/undo
- POST /eapi/v1/chore/:id/reject
- POST /eapi/v1/chore/:id/not-needed
- PUT /eapi/v1/chore/:id
- GET /eapi/v1/circle/members

**Error when not Plus member:**
```json
{"error": "Only plus members can access this endpoint"}
```

---

## Rate Limiting

EAPI endpoints are rate-limited to prevent abuse.

**Default limits:**
- 300 requests per minute per token
- Configurable via `DT_SERVER_RATE_LIMIT` and `DT_SERVER_RATE_PERIOD`

**Rate limit headers:**
```http
X-RateLimit-Limit: 300
X-RateLimit-Remaining: 295
X-RateLimit-Reset: 1735841580
```

**Rate limit exceeded:**
```json
{
  "error": "Rate limit exceeded",
  "retryAfter": 45
}
```

---

## Common Errors

### 401 Unauthorized
```json
{"error": "API token required"}
{"error": "Invalid API token"}
{"error": "User account is disabled"}
```

### 403 Forbidden
```json
{"error": "Only plus members can access this endpoint"}
{"error": "MFA verification required"}
```

### 404 Not Found
```json
{"error": "Thing not found"}
{"error": "Chore not found"}
```

### 400 Bad Request
```json
{"error": "Invalid state value"}
{"error": "Invalid state for thing"}
{"error": "Invalid increment value"}
{"error": "Chore is out of completion window"}
```

### 500 Internal Server Error
```json
{"error": "Error completing chore"}
{"error": "Error getting chore history"}
{"error": "attempt to write a readonly database (8)"}  // Database permissions
```

---

## Thing Types and States

### Boolean
- **Type:** `boolean`
- **Valid states:** `"true"`, `"false"`
- **Example:** Dishwasher empty/full

### Numeric
- **Type:** `numeric`
- **Valid states:** Any integer as string (e.g., `"0"`, `"5"`, `"-3"`)
- **Example:** Days since last watering

### Text
- **Type:** `text`
- **Valid states:** Any string
- **Example:** Current mood, status

---

## Thing Actions & Triggers

When completing a chore, you can configure it to:

1. **Update Thing state** (Thing Action)
   - Example: Completing "Empty dishwasher" sets "Dishwasher empty" to `true`

2. **Schedule chores** (Thing Trigger)
   - Example: When "Dishwasher empty" becomes `false`, schedule "Empty dishwasher"

**Configuration:**
- Thing Actions: Set in chore settings (updates Thing when chore is completed)
- Thing Triggers: Set in Thing settings (schedules chores when state changes)

**Use Case Example:**
```
1. Complete chore "Load dishwasher"
   → Thing Action: Set "Dishwasher empty" = false

2. Thing state changes to false
   → Thing Trigger: Schedule "Empty dishwasher" chore

3. Complete chore "Empty dishwasher"
   → Thing Action: Set "Dishwasher empty" = true
```

---

## Example Integration

### Mobile App - Getting Dashboard Data

```javascript
const API_TOKEN = 'your_api_token_here';
const BASE_URL = 'https://donetick.example.com/eapi/v1';

async function fetchDashboard() {
  const headers = { 'secretkey': API_TOKEN };

  // Get things
  const things = await fetch(`${BASE_URL}/things/`, { headers })
    .then(r => r.json());

  // Get chores
  const chores = await fetch(`${BASE_URL}/chore`, { headers })
    .then(r => r.json());

  // Get points leaderboard
  const points = await fetch(`${BASE_URL}/chore/points`, { headers })
    .then(r => r.json());

  // Get recent history
  const history = await fetch(`${BASE_URL}/chore/history?limit=10`, { headers })
    .then(r => r.json());

  return { things, chores, points, history };
}

// Get single chore details
async function getChore(choreId) {
  const response = await fetch(`${BASE_URL}/chore/${choreId}`, {
    headers: { 'secretkey': API_TOKEN }
  });
  return response.json();
}

// Get archived chores
async function getArchivedChores() {
  const response = await fetch(`${BASE_URL}/chore/archived`, {
    headers: { 'secretkey': API_TOKEN }
  });
  return response.json();
}

// Complete a chore (simple - attributed to API token owner)
async function completeChore(choreId) {
  const response = await fetch(`${BASE_URL}/chore/${choreId}/complete`, {
    method: 'POST',
    headers: {
      'secretkey': API_TOKEN,
      'Content-Type': 'application/json'
    },
    body: '{}'
  });

  return response.json();
}

// Complete a chore (specify who completed it)
async function completeChoreAs(choreId, userId) {
  const response = await fetch(`${BASE_URL}/chore/${choreId}/complete`, {
    method: 'POST',
    headers: {
      'secretkey': API_TOKEN,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ completedBy: userId })
  });

  return response.json();
}

// Toggle thing state
async function toggleThing(thingId, newState) {
  const response = await fetch(
    `${BASE_URL}/things/${thingId}/state/change?set=${newState}`,
    { headers: { 'secretkey': API_TOKEN } }
  );

  return response.json();
}
```

---

## Changelog

### v1.0.4-fork (2026-01-18)
- **Added:** `POST /eapi/v1/chore/:id/undo` - Undo last chore completion
- **Added:** `POST /eapi/v1/chore/:id/reject` - Reject pending approval chores (admin only)
- **Added:** `POST /eapi/v1/chore/:id/not-needed` - Mark chore as not needed
- **Added:** New chore history statuses: AUTO_SKIPPED (6), NOT_NEEDED (7)
- **Added:** Automatic 50% point penalty for late completions (after due date)
- **Added:** Daily auto-reschedule for overdue recurring chores

### v1.0.3-fork (2026-01-03)
- **Fixed:** `POST /eapi/v1/chore/:id/complete` now reads `completedBy` from JSON request body
- **Changed:** `completedBy` priority: JSON body → query parameter → API token owner
- **Backward compatible:** Query parameter `?completedBy=X` still works as fallback

### v1.0.2-fork (2026-01-02)
- **Fixed:** CORS issue on `/eapi/v1/things` - now supports both with and without trailing slash
- **Added:** `GET /eapi/v1/chore/:id` - Get single chore by ID
- **Added:** `GET /eapi/v1/chore/archived` - Get archived chores

### v1.0.1-fork (2026-01-02)
- **Fixed:** Frontend date format bug in UserActivities.jsx causing month/day swap
- **Fixed:** "Invalid date" errors in activity history display

### v1.0.0-fork (2026-01-02)
- Initial External API implementation
- Things endpoints (list, get, state update, state change)
- Chore endpoints (list, history, points, create, complete, update, delete)
- Circle members endpoint
- API token authentication
- Plus member access control
- Rate limiting support
- Thing actions and triggers integration

---

## Support

For issues or questions:
- GitHub Issues: https://github.com/tomcer/donetick-fork/issues
- Documentation: https://github.com/tomcer/donetick-fork/blob/main/EXTERNAL_API.md
