# Thing Actions Feature - Implementation Documentation

## Overview

Thing Actions allow automatic modification of Thing states when chores are completed. This enables automation workflows like "Water plants" automatically updating a "Plants watered" Thing to true.

## Implementation Summary

**Feature Branch:** `feature/thing-actions`
**Commits:** 3 atomic commits
**Status:** ✅ Implemented, ready for testing

### Commits

1. **7e3df58** - Model update (ActionType, ActionValue fields)
2. **d0ca155** - Database migration
3. **6cc0683** - Handler implementation

## Changes Made

### 1. Model Update
**File:** `internal/thing/model/model.go`

Added fields to `ThingChore` struct:
```go
ActionType   string `json:"actionType,omitempty" gorm:"column:action_type"`
ActionValue  string `json:"actionValue,omitempty" gorm:"column:action_value"`
```

### 2. Database Migration
**File:** `migrations/20251228_add_thing_action_fields.go`

- Adds `action_type` and `action_value` columns to `thing_chores` table
- Includes rollback support (Down method)
- Safe column addition with existence checks

### 3. Handler Implementation
**File:** `internal/chore/handler.go`

**New Function:** `executeThingAction()`
- Location: Lines 3242-3323
- Executes after chore completion, before notifications
- Type-safe action execution

**Modified Function:** `completeChore()`
- Location: Lines 1702-1710
- Calls `executeThingAction()` after subtask reset

## Supported Actions

### 1. Set
**Type:** `"set"`
**Description:** Set Thing to specific value
**ActionValue:** Target value
**Example:**
```json
{
  "actionType": "set",
  "actionValue": "completed"
}
```

### 2. Toggle
**Type:** `"toggle"`
**Description:** Toggle boolean Thing (true ↔ false)
**ActionValue:** Not used
**Constraints:** Thing.Type must be "boolean"
**Example:**
```json
{
  "actionType": "toggle",
  "actionValue": ""
}
```

### 3. Increment
**Type:** `"increment"`
**Description:** Increase number Thing by amount
**ActionValue:** Increment amount (default: 1)
**Constraints:** Thing.Type must be "number"
**Example:**
```json
{
  "actionType": "increment",
  "actionValue": "5"
}
```

### 4. Decrement
**Type:** `"decrement"`
**Description:** Decrease number Thing by amount
**ActionValue:** Decrement amount (default: 1)
**Constraints:** Thing.Type must be "number"
**Example:**
```json
{
  "actionType": "decrement",
  "actionValue": "1"
}
```

## Testing

### Build & Run

```bash
cd /data/projects/tomcer/donetic_fork/backend

# Checkout feature branch
git checkout feature/thing-actions

# Build
go build -o donetick .

# Run with development config
export DT_ENV=selfhosted
export DT_JWT_SECRET="test_secret_minimum_32_characters_long"
./donetick
```

### Create Test Data

```bash
BASE_URL="http://localhost:2021/api/v1"

# 1. Login (replace with your credentials)
TOKEN=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' | jq -r '.token')

# 2. Create a Thing
THING_RESPONSE=$(curl -s -X POST "$BASE_URL/things" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Plants Watered",
    "type": "boolean",
    "state": "false"
  }')

THING_ID=$(echo $THING_RESPONSE | jq -r '.res.id')
echo "Created Thing ID: $THING_ID"

# 3. Create Chore with Thing Action
CHORE_RESPONSE=$(curl -s -X POST "$BASE_URL/chores" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"Water Plants\",
    \"frequencyType\": \"daily\",
    \"assignStrategy\": \"least_completed\",
    \"thingChore\": {
      \"thingId\": $THING_ID,
      \"actionType\": \"toggle\",
      \"actionValue\": \"\"
    }
  }")

CHORE_ID=$(echo $CHORE_RESPONSE | jq -r '.res.id')
echo "Created Chore ID: $CHORE_ID"

# 4. Complete the Chore
curl -s -X POST "$BASE_URL/chores/$CHORE_ID/do" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}' | jq

# 5. Verify Thing state changed
curl -s "$BASE_URL/things/$THING_ID" \
  -H "Authorization: Bearer $TOKEN" | jq '.res.state'

# Expected output: "true"
```

### Test All Action Types

#### Test Increment
```bash
# Create number Thing
curl -s -X POST "$BASE_URL/things" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Counter",
    "type": "number",
    "state": "0"
  }' | jq

# Create chore with increment action
curl -s -X POST "$BASE_URL/chores" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Increment Counter",
    "frequencyType": "daily",
    "thingChore": {
      "thingId": <THING_ID>,
      "actionType": "increment",
      "actionValue": "5"
    }
  }' | jq
```

#### Test Set
```bash
# Create text Thing
curl -s -X POST "$BASE_URL/things" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Status",
    "type": "text",
    "state": "pending"
  }' | jq

# Create chore with set action
curl -s -X POST "$BASE_URL/chores" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Complete Task",
    "frequencyType": "once",
    "thingChore": {
      "thingId": <THING_ID>,
      "actionType": "set",
      "actionValue": "completed"
    }
  }' | jq
```

### Watch Logs

```bash
# In separate terminal, watch for action execution
tail -f donetick.log | grep -i "thing action\|executed thing"

# Expected log output:
# {"level":"info","ts":"...","msg":"Executed Thing action",
#  "choreId":1,"thingId":1,"actionType":"toggle",
#  "oldState":"false","newState":"true"}
```

## Docker Testing

```bash
cd /data/projects/tomcer/donetic_fork/backend

# Build Docker image
docker build -t ghcr.io/tomcer/donetick:thing-actions .

# Run container
docker run -d \
  --name donetick-thing-actions \
  -p 2021:2021 \
  -v $(pwd)/dev-data:/usr/src/app/data \
  -e DT_ENV=selfhosted \
  -e DT_JWT_SECRET="test_secret_minimum_32_characters_long" \
  ghcr.io/tomcer/donetick:thing-actions

# Check logs
docker logs -f donetick-thing-actions

# Stop
docker stop donetick-thing-actions
docker rm donetick-thing-actions
```

## Error Handling

### Non-Blocking Errors
Action execution failures are logged as warnings but don't block chore completion:

```go
if err := h.executeThingAction(c, updatedChore, logger); err != nil {
    logger.Warn("Failed to execute Thing action", ...)
}
```

### Type Validation
Actions validate Thing types before execution:
- Toggle only works on boolean Things
- Increment/Decrement only work on number Things

### Safe Defaults
- If ActionValue is empty for increment/decrement, defaults to 1
- Toggle handles empty state as "false"

## Integration Points

### Execution Flow
1. User completes chore via API: `POST /api/v1/chores/:id/do`
2. `completeChore()` handler processes completion
3. `choreRepo.CompleteChore()` updates database
4. Subtasks reset (if applicable)
5. **→ Thing action executes here** ←
6. Notifications generated
7. Real-time broadcast
8. Response sent

### Dependencies
- Uses existing `ThingRepository` (no new dependencies)
- Leverages `logging.Logger` for output
- Integrates with existing error handling

## Code Quality

✅ Follows existing code patterns
✅ Type-safe operations
✅ Graceful error handling
✅ Detailed logging
✅ Backward compatible
✅ Zero breaking changes

## Merge Strategy

### To Develop
```bash
git checkout develop
git merge feature/thing-actions
git push origin develop
```

### To Main (after testing)
```bash
git checkout main
git merge develop
git push origin main
```

### Upstream PR (optional)
1. Test thoroughly
2. Add unit tests (optional)
3. Create PR to `donetick/donetick`
4. Describe use case and benefits

## Use Cases

### Home Automation
- "Turn off lights" chore → Set "Lights" Thing to "off"
- "Water plants" chore → Toggle "Plants watered" to true

### Habit Tracking
- "Exercise" chore → Increment "Exercise streak" counter
- "Skip workout" chore → Reset "Streak" to 0

### Task Management
- "Complete project phase" chore → Set "Project status" to "phase2"
- "Review document" chore → Toggle "Needs review" to false

## Known Limitations

1. Actions execute synchronously (blocking)
2. No action history/audit trail
3. No conditional actions (always executes)
4. Single action per chore (no chaining)

## Future Enhancements

- Action history logging
- Conditional action execution
- Action chaining support
- Async action execution
- Rollback support on chore undo

## Troubleshooting

### Action Not Executing

**Check:**
1. Thing exists and has correct ID
2. ActionType is valid ("set", "toggle", "increment", "decrement")
3. Thing type matches action (toggle → boolean, increment → number)
4. Check logs for warnings

**Debug:**
```bash
# Enable debug logging
export DT_LOG_LEVEL=debug

# Check Thing state before and after
curl "$BASE_URL/things/$THING_ID" -H "Authorization: Bearer $TOKEN" | jq '.res.state'
```

### Build Errors

```bash
# Clean build
rm -f donetick
go clean
go mod tidy
go build -o donetick .
```

### Migration Issues

```bash
# Check migration status in logs
grep -i "migration\|thing_chores" donetick.log

# Manually verify columns exist
sqlite3 data/donetick.db ".schema thing_chores"
```

## Files Modified

```
backend/
├── internal/
│   ├── thing/model/model.go           (modified - 2 lines added)
│   └── chore/handler.go               (modified - 93 lines added)
└── migrations/
    └── 20251228_add_thing_action_fields.go  (new - 82 lines)
```

## Contact

For questions or issues:
- GitHub: https://github.com/tomcer/donetick/issues
- Feature branch: https://github.com/tomcer/donetick/tree/feature/thing-actions
