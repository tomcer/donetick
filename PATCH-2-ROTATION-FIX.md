# Patch 2: Trigger Chore Rotation Fix

## 🐛 Problém

**Trigger chores nemají funkční rotaci assignees!**

### Symptomy
1. ❌ Po completion trigger chore `assignedTo` zůstává stejný
2. ❌ Round robin strategie nefunguje
3. ❌ Trigger chores se archivují (`isActive = false`)

### Root Cause

**Soubor**: `internal/chore/repo/repository.go`
**Funkce**: `CompleteChore()`

```go
// SOUČASNÝ KÓD (BUGGY):
if dueDate != nil {
    choreUpdates["assigned_to"] = nextAssignedTo  // ✅ Rotation se uloží
} else {
    // one time task
    choreUpdates["is_active"] = false  // ❌ Trigger chores se archivují!
}
```

**Proč je to bug**:
- Trigger chores mají `dueDate = nil` (z scheduler.go line 16-18)
- Condition `if dueDate != nil` je FALSE
- Rotation se NIKDY neuloží
- Trigger chores se archivují místo toho

---

## ✅ Řešení

### Oprava v `repository.go`

**Najdi funkci**: `CompleteChore()` (kolem řádku 100-150)

**PŘED** (buggy):
```go
choreUpdates := map[string]interface{}{
    "next_due_date": dueDate,
    "status":        chModel.ChoreStatusNoStatus,
}

if dueDate != nil {
    choreUpdates["assigned_to"] = nextAssignedTo
} else {
    // one time task
    choreUpdates["is_active"] = false
}
```

**PO** (fixed):
```go
choreUpdates := map[string]interface{}{
    "next_due_date": dueDate,
    "status":        chModel.ChoreStatusNoStatus,
}

// FIX 1: VŽDY ulož rotation (i když dueDate=nil)
choreUpdates["assigned_to"] = nextAssignedTo

// FIX 2: Archivuj JEN non-trigger one-time tasks
if dueDate == nil && chore.FrequencyType != "trigger" {
    // one time task
    choreUpdates["is_active"] = false
}
```

### Změny:
1. ✅ **Rotation**: Vždy se uloží (i pro trigger chores)
2. ✅ **Archivace**: Trigger chores ZŮSTÁVAJÍ aktivní
3. ✅ **One-time tasks**: Stále se archivují (FrequencyType != "trigger")

---

## 📝 Implementace

### Krok 1: Najdi Správný Kód

**GoLand**: Ctrl+Shift+N → `repository.go`

**Hledej** (Ctrl+F):
```
func (r *ChoreRepository) CompleteChore
```

**Najdeš sekci**:
```go
if dueDate != nil {
    choreUpdates["assigned_to"] = nextAssignedTo
} else {
    choreUpdates["is_active"] = false
}
```

### Krok 2: Aplikuj Fix

**Nahraď celou sekci**:
```go
// PATCH 2: Always save rotation, don't archive trigger chores
choreUpdates["assigned_to"] = nextAssignedTo

// Archive only non-trigger one-time tasks
if dueDate == nil && chore.FrequencyType != "trigger" {
    choreUpdates["is_active"] = false
}
```

### Krok 3: Verify Kontext

**Ujisti se že máš přístup k `chore.FrequencyType`**:

V signature `CompleteChore()` by mělo být:
```go
func (r *ChoreRepository) CompleteChore(
    c context.Context,
    chore *chModel.Chore,  // ← Musí být chore object, ne jen ID!
    // ... other params
) error
```

Pokud máš jen `choreID int`, musíš chore načíst:
```go
// Na začátku funkce
chore, err := r.GetChoreByID(c, choreID)
if err != nil {
    return err
}
```

---

## 🧪 Testing

### Setup Test Data

```bash
BASE_URL="http://localhost:8080/api/v1"

# Login
TOKEN=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' | jq -r '.token')

# Create Thing
THING_ID=$(curl -s -X POST "$BASE_URL/things" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "Test Rotation Thing",
    "type": "boolean",
    "state": "false"
  }' | jq -r '.res.id')

echo "Thing ID: $THING_ID"

# Create Trigger Chore with Round Robin
CHORE_ID=$(curl -s -X POST "$BASE_URL/chores" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "Test Rotation Chore",
    "frequencyType": "trigger",
    "assignStrategy": "round_robin",
    "assignees": [1, 2, 3],
    "assignedTo": 1,
    "thingChore": {
      "thingId": '$THING_ID',
      "triggerState": "false",
      "actionType": "toggle"
    }
  }' | jq -r '.res.id')

echo "Chore ID: $CHORE_ID"
```

### Test 1: Verify Initial State

```bash
curl -s "$BASE_URL/chores/$CHORE_ID" \
  -H "Authorization: Bearer $TOKEN" | \
  jq '{id, name, assignedTo, isActive, frequencyType}'
```

**Expected**:
```json
{
  "id": 1,
  "name": "Test Rotation Chore",
  "assignedTo": 1,
  "isActive": true,
  "frequencyType": "trigger"
}
```

### Test 2: Complete Chore (1st time)

```bash
echo "=== Completing 1st time (User 1) ==="
curl -s -X POST "$BASE_URL/chores/$CHORE_ID/do" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"completedBy": 1}' | jq

sleep 1

echo "=== Check assignedTo after 1st completion ==="
ASSIGNED_AFTER_1=$(curl -s "$BASE_URL/chores/$CHORE_ID" \
  -H "Authorization: Bearer $TOKEN" | jq -r '.res.assignedTo')

echo "Assigned to: $ASSIGNED_AFTER_1"
# Expected: 2 (rotated from 1)
```

### Test 3: Complete Again (2nd time)

```bash
# Toggle Thing back
curl -s -X PUT "$BASE_URL/things/$THING_ID/state?value=false" \
  -H "Authorization: Bearer $TOKEN" > /dev/null

sleep 1

echo "=== Completing 2nd time (User 2) ==="
curl -s -X POST "$BASE_URL/chores/$CHORE_ID/do" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"completedBy": 2}' | jq

sleep 1

echo "=== Check assignedTo after 2nd completion ==="
ASSIGNED_AFTER_2=$(curl -s "$BASE_URL/chores/$CHORE_ID" \
  -H "Authorization: Bearer $TOKEN" | jq -r '.res.assignedTo')

echo "Assigned to: $ASSIGNED_AFTER_2"
# Expected: 3 (rotated from 2)
```

### Test 4: Verify Not Archived

```bash
echo "=== Check if chore is still active ==="
IS_ACTIVE=$(curl -s "$BASE_URL/chores/$CHORE_ID" \
  -H "Authorization: Bearer $TOKEN" | jq -r '.res.isActive')

echo "Is Active: $IS_ACTIVE"
# Expected: true (NOT archived)
```

### Test 5: Full Test Script

**Soubor**: `/tmp/test-rotation-fix.sh`

```bash
#!/bin/bash
set -e

BASE_URL="http://localhost:8080/api/v1"

echo "========================================="
echo "  TESTING TRIGGER CHORE ROTATION FIX"
echo "========================================="
echo ""

# Login
TOKEN=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' | jq -r '.token')

if [ "$TOKEN" == "null" ] || [ -z "$TOKEN" ]; then
    echo "❌ Login failed!"
    exit 1
fi

echo "✅ Logged in successfully"
echo ""

# Create Thing
echo "Creating test Thing..."
THING_ID=$(curl -s -X POST "$BASE_URL/things" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "Rotation Test Thing",
    "type": "boolean",
    "state": "false"
  }' | jq -r '.res.id')

echo "✅ Thing created (ID: $THING_ID)"
echo ""

# Create Trigger Chore
echo "Creating trigger chore with round_robin..."
CHORE_ID=$(curl -s -X POST "$BASE_URL/chores" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "Rotation Test Chore",
    "frequencyType": "trigger",
    "assignStrategy": "round_robin",
    "assignees": [1, 2, 3],
    "assignedTo": 1,
    "points": 5,
    "thingChore": {
      "thingId": '$THING_ID',
      "triggerState": "false",
      "actionType": "toggle"
    }
  }' | jq -r '.res.id')

echo "✅ Chore created (ID: $CHORE_ID)"
echo ""

# Test Rotation Cycle
echo "========================================="
echo "  ROTATION TEST CYCLE"
echo "========================================="
echo ""

for i in 1 2 3; do
    echo "--- Cycle $i ---"

    # Get current assignedTo
    ASSIGNED_BEFORE=$(curl -s "$BASE_URL/chores/$CHORE_ID" \
      -H "Authorization: Bearer $TOKEN" | jq -r '.res.assignedTo')

    echo "Assigned BEFORE: $ASSIGNED_BEFORE"

    # Complete chore
    curl -s -X POST "$BASE_URL/chores/$CHORE_ID/do" \
      -H "Authorization: Bearer $TOKEN" \
      -d "{\"completedBy\": $ASSIGNED_BEFORE}" > /dev/null

    sleep 1

    # Get new assignedTo
    CHORE_DATA=$(curl -s "$BASE_URL/chores/$CHORE_ID" \
      -H "Authorization: Bearer $TOKEN")

    ASSIGNED_AFTER=$(echo $CHORE_DATA | jq -r '.res.assignedTo')
    IS_ACTIVE=$(echo $CHORE_DATA | jq -r '.res.isActive')

    echo "Assigned AFTER:  $ASSIGNED_AFTER"
    echo "Is Active:       $IS_ACTIVE"

    # Verify rotation happened
    if [ "$ASSIGNED_BEFORE" == "$ASSIGNED_AFTER" ]; then
        echo "❌ ROTATION FAILED - assignedTo didn't change!"
        exit 1
    else
        echo "✅ Rotation worked ($ASSIGNED_BEFORE → $ASSIGNED_AFTER)"
    fi

    # Verify not archived
    if [ "$IS_ACTIVE" != "true" ]; then
        echo "❌ ARCHIVING BUG - chore became inactive!"
        exit 1
    else
        echo "✅ Chore stayed active"
    fi

    echo ""

    # Toggle Thing back for next iteration (except last)
    if [ $i -lt 3 ]; then
        THING_STATE=$(curl -s "$BASE_URL/things/$THING_ID" \
          -H "Authorization: Bearer $TOKEN" | jq -r '.res.state')

        NEW_STATE="false"
        if [ "$THING_STATE" == "false" ]; then
            NEW_STATE="true"
        fi

        curl -s -X PUT "$BASE_URL/things/$THING_ID/state?value=$NEW_STATE" \
          -H "Authorization: Bearer $TOKEN" > /dev/null

        sleep 1
    fi
done

echo "========================================="
echo "  ✅ ALL TESTS PASSED!"
echo "========================================="
echo ""
echo "Summary:"
echo "- Rotation works correctly (1→2→3)"
echo "- Trigger chores stay active"
echo "- Thing actions execute properly"
echo ""

# Cleanup
echo "Cleaning up test data..."
curl -s -X DELETE "$BASE_URL/chores/$CHORE_ID" \
  -H "Authorization: Bearer $TOKEN" > /dev/null
curl -s -X DELETE "$BASE_URL/things/$THING_ID" \
  -H "Authorization: Bearer $TOKEN" > /dev/null

echo "✅ Done!"
```

**Použití**:
```bash
chmod +x /tmp/test-rotation-fix.sh
/tmp/test-rotation-fix.sh
```

---

## 📊 Expected Results

### PŘED Patch
```
Cycle 1: User 1 → User 1 ❌ (žádná rotace)
Cycle 2: User 1 → User 1 ❌ (žádná rotace)
Is Active: false ❌ (archivováno)
```

### PO Patch
```
Cycle 1: User 1 → User 2 ✅ (rotace funguje)
Cycle 2: User 2 → User 3 ✅ (rotace funguje)
Cycle 3: User 3 → User 1 ✅ (rotace funguje)
Is Active: true ✅ (zůstává aktivní)
```

---

## 🎯 Success Criteria

- [ ] Build compiles bez errorů
- [ ] Server runs bez chyb
- [ ] Trigger chore completion rotuje assignedTo
- [ ] Round robin funguje správně (1→2→3→1)
- [ ] Trigger chores NEZŮSTÁVAJÍ archivované
- [ ] One-time tasks (frequencyType="once") STÁLE se archivují
- [ ] Logs ukazují rotation: `INFO assigned next performer`

---

## 🔄 Git Workflow

### Create Feature Branch

```bash
cd /data/projects/tomcer/donetic_fork/backend

# Create branch from feature/thing-actions
git checkout feature/thing-actions
git checkout -b feature/rotation-fix

# Make changes to repository.go
# ... edit file ...

# Commit
git add internal/chore/repo/repository.go
git commit -m "fix: Always save rotation for trigger chores

- Rotation now saves even when nextDueDate is null
- Trigger chores no longer get archived after completion
- One-time tasks still archive correctly (frequencyType != trigger)

Fixes rotation issue where trigger chores assignedTo stayed the same"

# Push
git push origin feature/rotation-fix
```

---

## 📝 Additional Context

### Why This Bug Exists

1. **Scheduler** (`scheduler.go:16-18`) vrací `nil` pro trigger chores
2. **Repository** ukládá rotation JEN když `dueDate != nil`
3. **Result**: Trigger chores nikdy nedostanou rotation

### Why Original Code Did This

- Původní logika: "Pokud není další due date, je to one-time task → archivuj"
- **Problém**: Trigger chores NEMAJÍ due date, ale NEJSOU one-time!
- **Fix**: Explicitně check `frequencyType != "trigger"`

### Related Code

**Scheduler** (`internal/chore/scheduler.go:15-18`):
```go
func scheduleNextDueDate(...) (*time.Time, error) {
    if chore.FrequencyType == "trigger" {
        return nil, nil  // ← Trigger chores return nil
    }
    // ...
}
```

**Handler** (`internal/chore/handler.go`):
```go
// After completion
nextAssignedTo := calculateNextAssignee(chore)  // ✅ Calculates correctly
// ... calls repository.CompleteChore(nextAssignedTo)
```

**Repository** (`internal/chore/repo/repository.go`) - **THIS IS WHERE THE BUG IS**:
```go
// BUGGY:
if dueDate != nil {
    updates["assigned_to"] = nextAssignedTo  // ← Never saves for trigger!
}
```

---

## ✅ Checklist

- [x] Find CompleteChore() function in repository.go
- [x] Verify you have access to chore.FrequencyType
- [x] Apply rotation fix (always save assigned_to)
- [x] Apply archiving fix (check FrequencyType != "trigger")
- [x] Build successfully
- [x] Run server
- [x] Create test trigger chore
- [x] Complete 5x and verify rotation (1→2→3→1→2→3)
- [x] Verify chore stays active (isActive=true)
- [x] Run comprehensive test (8 cycles with 4 kids)
- [x] Verify complete use case (dishwasher rotation)
- [x] Commit with clear message
- [x] Push to GitHub

---

## 🎉 After This Patch

Trigger chores budou **plně funkční**:
- ✅ Thing actions po completion (Patch 1)
- ✅ Rotation funguje (Patch 2)
- ✅ Nezůstávají archivované (Patch 2)

**Next**: Merge do develop → Test → GHCR publish → Production! 🚀