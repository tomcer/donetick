package chore

import (
	"sort"
	"strconv"
	"time"

	"donetick.com/core/config"
	"donetick.com/core/internal/auth"
	authMiddleware "donetick.com/core/internal/auth"
	chRepo "donetick.com/core/internal/chore/repo"
	"donetick.com/core/internal/events"
	nps "donetick.com/core/internal/notifier/service"
	"donetick.com/core/internal/utils"
	"donetick.com/core/logging"
	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"

	limiter "github.com/ulule/limiter/v3"

	chModel "donetick.com/core/internal/chore/model"
	cRepo "donetick.com/core/internal/circle/repo"
	stRepo "donetick.com/core/internal/subtask/repo"
	tModel "donetick.com/core/internal/thing/model"
	tRepo "donetick.com/core/internal/thing/repo"
	uRepo "donetick.com/core/internal/user/repo"
)

type API struct {
	choreRepo     *chRepo.ChoreRepository
	userRepo      *uRepo.UserRepository
	circleRepo    *cRepo.CircleRepository
	nPlanner      *nps.NotificationPlanner
	eventProducer *events.EventsProducer
	stRepo        *stRepo.SubTasksRepository
	tRepo         *tRepo.ThingRepository
}

func NewAPI(cr *chRepo.ChoreRepository, userRepo *uRepo.UserRepository, circleRepo *cRepo.CircleRepository, nPlanner *nps.NotificationPlanner, eventProducer *events.EventsProducer, stRepo *stRepo.SubTasksRepository, tRepo *tRepo.ThingRepository) *API {
	return &API{
		choreRepo:     cr,
		userRepo:      userRepo,
		circleRepo:    circleRepo,
		nPlanner:      nPlanner,
		eventProducer: eventProducer,
		stRepo:        stRepo,
		tRepo:         tRepo,
	}
}

func (h *API) GetAllChores(c *gin.Context) {
	user := auth.MustCurrentUser(c)

	// Parse maxResults parameter
	maxResultsStr := c.DefaultQuery("maxResults", "100")
	maxResults, err := strconv.Atoi(maxResultsStr)
	if err != nil || maxResults <= 0 || maxResults > 500 {
		maxResults = 100
	}

	// Get all chores
	chores, err := h.choreRepo.GetChores(c, user.CircleID, user.ID, false)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// Get all things for filtering
	var things []struct {
		ID    int
		State string
	}
	if err := h.choreRepo.GetDB().WithContext(c).
		Table("things").
		Select("id, state").
		Where("user_id = ?", user.ID).
		Find(&things).Error; err != nil {
		c.JSON(500, gin.H{"error": "Failed to fetch things for filtering"})
		return
	}

	// Create a map of thing ID -> state for quick lookup
	thingStates := make(map[int]string)
	for _, thing := range things {
		thingStates[thing.ID] = thing.State
	}

	// Filter chores based on Thing trigger conditions
	var filteredChores []*chModel.Chore
	for _, chore := range chores {
		// If no thing trigger, include the chore
		if chore.ThingChore == nil {
			filteredChores = append(filteredChores, chore)
			continue
		}

		// Check if thing trigger condition is met
		thingState, exists := thingStates[chore.ThingChore.ThingID]
		if !exists {
			// Thing not found, skip this chore
			continue
		}

		// Check if trigger condition is satisfied
		if isThingTriggerSatisfied(thingState, chore.ThingChore.TriggerState, chore.ThingChore.Condition) {
			filteredChores = append(filteredChores, chore)
		}
	}

	// Apply maxResults limit
	if len(filteredChores) > maxResults {
		filteredChores = filteredChores[:maxResults]
	}

	// Ensure we return empty array [] instead of null when no chores
	if filteredChores == nil {
		filteredChores = []*chModel.Chore{}
	}

	c.JSON(200, filteredChores)
}

// isThingTriggerSatisfied checks if a thing's current state satisfies the trigger condition
func isThingTriggerSatisfied(currentState, triggerState, condition string) bool {
	// For boolean and text types, simple equality check
	if condition == "" {
		return currentState == triggerState
	}

	// For number types with conditions (eq, neq, gt, gte, lt, lte)
	currentValue, err1 := strconv.ParseFloat(currentState, 64)
	triggerValue, err2 := strconv.ParseFloat(triggerState, 64)
	if err1 != nil || err2 != nil {
		// If not numbers, fall back to string comparison
		return currentState == triggerState
	}

	switch condition {
	case "eq":
		return currentValue == triggerValue
	case "neq":
		return currentValue != triggerValue
	case "gt":
		return currentValue > triggerValue
	case "gte":
		return currentValue >= triggerValue
	case "lt":
		return currentValue < triggerValue
	case "lte":
		return currentValue <= triggerValue
	default:
		return currentState == triggerState
	}
}

// GetChore returns a single chore by ID for External API
func (h *API) GetChore(c *gin.Context) {
	log := logging.FromContext(c)
	user := auth.MustCurrentUser(c)

	choreIDRaw := c.Param("id")
	choreID, err := strconv.Atoi(choreIDRaw)
	if err != nil {
		log.Debugw("chore.api.GetChore failed to parse chore ID", "error", err)
		c.JSON(400, gin.H{"error": "Invalid chore ID"})
		return
	}

	chore, err := h.choreRepo.GetChore(c, choreID, user.ID)
	if err != nil {
		log.Errorw("chore.api.GetChore failed to get chore", "error", err)
		c.JSON(404, gin.H{"error": "Chore not found"})
		return
	}

	c.JSON(200, chore)
}

// GetArchivedChores returns archived chores for External API
func (h *API) GetArchivedChores(c *gin.Context) {
	log := logging.FromContext(c)
	user := auth.MustCurrentUser(c)

	archivedChores, err := h.choreRepo.GetArchivedChores(c, user.CircleID, user.ID)
	if err != nil {
		log.Errorw("chore.api.GetArchivedChores failed to get archived chores", "error", err)
		c.JSON(500, gin.H{"error": "Error fetching archived chores"})
		return
	}

	c.JSON(200, gin.H{"res": archivedChores})
}

func (h *API) CreateChore(c *gin.Context) {
	log := logging.FromContext(c)
	var choreRequest chModel.ChoreLiteReq
	user := auth.MustCurrentUser(c)

	if err := c.BindJSON(&choreRequest); err != nil {
		log.Debugw("chore.api.CreateChore failed to bind JSON", "error", err)
		c.JSON(400, gin.H{"error": "Invalid request body"})
		return
	}

	// Validate required fields
	if choreRequest.Name == "" {
		c.JSON(400, gin.H{"error": "Chore name is required"})
		return
	}

	// Parse due date if provided
	var nextDueDate *time.Time
	if choreRequest.DueDate != "" {
		parsedDate, err := time.Parse(time.RFC3339, choreRequest.DueDate)
		if err != nil {
			parsedDateSimple, errSimple := time.Parse("2006-01-02", choreRequest.DueDate)
			if errSimple != nil {
				c.JSON(400, gin.H{"error": "Invalid due date format. Use RFC3339 or YYYY-MM-DD"})
				return
			}
			// Set time to now UTC
			now := time.Now().UTC()
			parsedDate = time.Date(parsedDateSimple.Year(), parsedDateSimple.Month(), parsedDateSimple.Day(), now.Hour(), now.Minute(), now.Second(), 0, time.UTC)
			err = nil
		}
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid due date format. Use RFC3339 format"})
			return
		}
		nextDueDate = &parsedDate
	}
	// get all circle members:
	circleUsers, err := h.circleRepo.GetCircleUsers(c, user.CircleID)
	if err != nil {
		log.Errorw("chore.api.CreateChore failed to get circle users", "error", err)
		c.JSON(500, gin.H{"error": "Failed to get circle members"})
		return
	}
	createdBy := user.ID
	if choreRequest.CreatedBy != nil {
		// Check if the specified user exists in the circle
		var found bool
		for _, u := range circleUsers {
			if u.UserID == *choreRequest.CreatedBy {
				found = true
				createdBy = u.UserID
				break
			}
		}
		if !found {
			log.Errorw("chore.api.CreateChore specified user not found in circle", "userID", *choreRequest.CreatedBy)
			c.JSON(400, gin.H{"error": "Specified user not found in circle"})
			return
		}
	}

	chore := &chModel.Chore{
		CreatedBy:     createdBy,
		CircleID:      user.CircleID,
		Name:          choreRequest.Name,
		IsActive:      true,
		FrequencyType: chModel.FrequencyTypeOnce,
		// Frequency:                choreRequest.Frequency,
		AssignStrategy: chModel.AssignmentStrategyRandom,
		AssignedTo:     &createdBy,
		Assignees:      []chModel.ChoreAssignees{{UserID: createdBy}},
		Description:    choreRequest.Description,
		NextDueDate:    nextDueDate,
		CreatedAt:      time.Now().UTC(),
	}

	id, err := h.choreRepo.CreateChore(c, chore)
	if err != nil {
		log.Errorw("chore.api.CreateChore failed to create chore", "error", err)
		c.JSON(500, gin.H{"error": "Error creating chore"})
		return
	}

	// Fetch the created chore with all relations
	createdChore, err := h.choreRepo.GetChore(c, id, user.ID)
	if err != nil {
		log.Errorw("chore.api.CreateChore failed to fetch created chore", "error", err)
		c.JSON(500, gin.H{"error": "Error fetching created chore"})
		return
	}

	c.JSON(201, createdChore)
}

func (h *API) UpdateChore(c *gin.Context) {
	log := logging.FromContext(c)
	var choreRequest chModel.ChoreLiteReq
	user := auth.MustCurrentUser(c)

	choreIDRaw := c.Param("id")
	choreID, err := strconv.Atoi(choreIDRaw)
	if err != nil {
		log.Debugw("chore.api.UpdateChore failed to parse chore ID", "error", err)
		c.JSON(400, gin.H{"error": "Invalid chore ID"})
		return
	}

	if err := c.BindJSON(&choreRequest); err != nil {
		log.Debugw("chore.api.UpdateChore failed to bind JSON", "error", err)
		c.JSON(400, gin.H{"error": "Invalid request body"})
		return
	}

	// Get existing chore
	existingChore, err := h.choreRepo.GetChore(c, choreID, user.ID)
	if err != nil {
		log.Errorw("chore.api.UpdateChore failed to get chore", "error", err)
		c.JSON(404, gin.H{"error": "Chore not found"})
		return
	}
	// get circle members:
	circleUsers, err := h.circleRepo.GetCircleUsers(c, user.CircleID)
	if err != nil {
		log.Errorw("chore.api.UpdateChore failed to get circle users", "error", err)
		c.JSON(500, gin.H{"error": "Failed to get circle members"})
		return
	}
	// Check if user owns this chore
	now := time.Now().UTC()
	if err := existingChore.CanEdit(user.ID, circleUsers, &now); err != nil {
		log.Debugw("chore.api.UpdateChore user does not own chore", "userID", user.ID, "choreCreatedBy", existingChore.CreatedBy)
		c.JSON(403, gin.H{"error": "You can only update your own chores"})
		return
	}

	// Validate required fields
	if choreRequest.Name == "" {
		c.JSON(400, gin.H{"error": "Chore name is required"})
		return
	}

	// Parse due date if provided
	var nextDueDate *time.Time
	if choreRequest.DueDate != "" {

		parsedDate, err := time.Parse(time.RFC3339, choreRequest.DueDate)
		if err != nil {
			parsedDateSimple, errSimple := time.Parse("2006-01-02", choreRequest.DueDate)
			if errSimple != nil {
				c.JSON(400, gin.H{"error": "Invalid due date format. Use RFC3339 or YYYY-MM-DD"})
				return
			}
			// Set time to now UTC
			now := time.Now().UTC()
			parsedDate = time.Date(parsedDateSimple.Year(), parsedDateSimple.Month(), parsedDateSimple.Day(), now.Hour(), now.Minute(), now.Second(), 0, time.UTC)
			err = nil
		}
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid due date format. Use RFC3339 format"})
			return
		}
		nextDueDate = &parsedDate
	}

	// Update only name and due date
	updates := map[string]interface{}{
		"name":          choreRequest.Name,
		"description":   choreRequest.Description,
		"next_due_date": nextDueDate,
		"updated_by":    user.ID,
		"updated_at":    time.Now().UTC(),
	}

	err = h.choreRepo.UpdateChoreFields(c, choreID, updates)
	if err != nil {
		log.Errorw("chore.api.UpdateChore failed to update chore", "error", err)
		c.JSON(500, gin.H{"error": "Error updating chore"})
		return
	}

	// Fetch the updated chore
	updatedChore, err := h.choreRepo.GetChore(c, choreID, user.ID)
	if err != nil {
		log.Errorw("chore.api.UpdateChore failed to fetch updated chore", "error", err)
		c.JSON(500, gin.H{"error": "Error fetching updated chore"})
		return
	}

	c.JSON(200, updatedChore)
}

func (h *API) CompleteChore(c *gin.Context) {
	log := logging.FromContext(c)
	completedDate := time.Now().UTC()
	choreIDRaw := c.Param("id")
	choreID, err := strconv.Atoi(choreIDRaw)
	if err != nil {
		log.Debugw("chore.api.CompleteChore failed to parse chore ID", "error", err)
		c.JSON(400, gin.H{
			"error": "Invalid ID",
		})
		return
	}

	// Support completedBy and custom points from both request body (JSON) and query parameter
	type CompleteRequest struct {
		CompletedBy int  `json:"completedBy"`
		Points      *int `json:"points"` // Optional: override chore points for this completion
	}

	currentUser := auth.MustCurrentUser(c)
	performer := currentUser.ID

	// Try to read from request body first (for POST with JSON)
	var req CompleteRequest
	if err := c.ShouldBindJSON(&req); err == nil && req.CompletedBy > 0 {
		log.Debugw("chore.api.CompleteChore completedBy from request body", "completedBy", req.CompletedBy)
		performer = req.CompletedBy
	} else if completedByRaw := c.Query("completedBy"); completedByRaw != "" {
		// Fallback to query parameter (for backward compatibility)
		if completedBy, err := strconv.Atoi(completedByRaw); err == nil && completedBy > 0 {
			log.Debugw("chore.api.CompleteChore completedBy from query param", "completedBy", completedBy)
			performer = completedBy
		}
	}
	chore, err := h.choreRepo.GetChore(c, choreID, currentUser.ID)
	if err != nil {
		log.Errorw("chore.api.CompleteChore failed to get chore", "error", err)
		c.JSON(500, gin.H{
			"error": "Error getting chore",
		})
		return
	}

	// user need to be assigned to the chore to complete it
	circleUsers, err := h.circleRepo.GetCircleUsers(c, currentUser.CircleID)
	if err != nil {
		log.Errorw("Failed to retrieve circle users", "error", err)
		c.JSON(500, gin.H{
			"error": "Failed to retrieve circle users",
		})
		return
	}
	if !chore.CanComplete(performer, circleUsers) {
		log.Debugw("chore.api.CompleteChore user is not assigned to chore", "userID", performer, "choreID", choreID)
		c.JSON(400, gin.H{
			"error": "User is not assigned to chore",
		})
		return
	}

	// confirm that the chore in completion window:
	if chore.CompletionWindow != nil {
		if completedDate.Before(chore.NextDueDate.Add(time.Hour * time.Duration(*chore.CompletionWindow))) {
			log.Debugw("chore.api.CompleteChore chore is in completion window", "choreID", choreID, "completionWindow", chore.CompletionWindow)
			c.JSON(400, gin.H{
				"error": "Chore is out of completion window",
			})
			return
		}
	}

	var nextDueDate *time.Time
	if chore.FrequencyType == "adaptive" {
		history, err := h.choreRepo.GetChoreHistoryWithLimit(c, chore.ID, 5)
		if err != nil {
			c.JSON(500, gin.H{
				"error": "Error getting chore history",
			})
			return
		}
		nextDueDate, err = scheduleAdaptiveNextDueDate(chore, completedDate, history)
		if err != nil {
			log.Debugw("chore.api.CompleteChore failed to schedule adaptive next due date", "error", err)

			c.JSON(500, gin.H{
				"error": "Error scheduling next due date",
			})
			return
		}

	} else {
		nextDueDate, err = scheduleNextDueDate(c, chore, completedDate.UTC())
		if err != nil {
			log.Debugw("chore.api.CompleteChore failed to schedule next due date", "error", err)
			c.JSON(500, gin.H{
				"error": "Error scheduling next due date",
			})
			return
		}
	}
	choreHistory, err := h.choreRepo.GetChoreHistory(c, chore.ID)
	if err != nil {
		c.JSON(500, gin.H{
			"error": "Error getting chore history",
		})
		return
	}

	nextAssignedTo, err := checkNextAssignee(chore, choreHistory, performer)
	if err != nil {
		log.Debugw("chore.api.CompleteChore failed to check next assignee", "error", err)
		c.JSON(500, gin.H{
			"error": "Error checking next assignee",
		})
		return
	}

	if err := h.choreRepo.CompleteChore(c, chore, nil, performer, nextDueDate, &completedDate, nextAssignedTo, true, req.Points); err != nil {
		c.JSON(500, gin.H{
			"error": "Error completing chore",
		})
		return
	}
	if chore.SubTasks != nil && chore.FrequencyType != chModel.FrequencyTypeOnce {
		h.stRepo.ResetSubtasksCompletion(c, chore.ID)
	}

	updatedChore, err := h.choreRepo.GetChore(c, choreID, currentUser.ID)
	if err != nil {
		c.JSON(500, gin.H{
			"error": "Error getting chore",
		})
		return
	}

	// Execute Thing action if configured
	if updatedChore.ThingChore != nil && updatedChore.ThingChore.ThingID > 0 && updatedChore.ThingChore.ActionType != "" {
		if err := h.executeThingAction(c, updatedChore); err != nil {
			log := logging.FromContext(c)
			log.Warnw("Failed to execute Thing action",
				"choreId", updatedChore.ID,
				"thingId", updatedChore.ThingChore.ThingID,
				"error", err)
		}
	}

	h.nPlanner.GenerateNotifications(c, updatedChore)
	h.eventProducer.ChoreCompleted(c, currentUser.WebhookURL, chore, &currentUser.User)
	c.JSON(200,
		updatedChore,
	)
}

func (h *API) UndoChoreCompletion(c *gin.Context) {
	log := logging.FromContext(c)
	choreIDRaw := c.Param("id")
	choreID, err := strconv.Atoi(choreIDRaw)
	if err != nil {
		log.Debugw("chore.api.UndoChoreCompletion failed to parse chore ID", "error", err)
		c.JSON(400, gin.H{"error": "Invalid ID"})
		return
	}

	// Request body with optional new due date
	type UndoRequest struct {
		NewDueDate *time.Time `json:"newDueDate"`
	}

	var req UndoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body
		req.NewDueDate = nil
	}

	currentUser := auth.MustCurrentUser(c)

	// Get the chore to verify access
	chore, err := h.choreRepo.GetChore(c, choreID, currentUser.ID)
	if err != nil {
		log.Errorw("chore.api.UndoChoreCompletion failed to get chore", "error", err)
		c.JSON(500, gin.H{"error": "Error getting chore"})
		return
	}

	// Get the latest completed history entry for this chore
	history, err := h.choreRepo.GetLatestChoreHistory(c, choreID)
	if err != nil {
		log.Errorw("chore.api.UndoChoreCompletion failed to get latest history", "error", err)
		c.JSON(500, gin.H{"error": "No completion to undo"})
		return
	}

	// Verify user has permission to undo (same as completion permission)
	circleUsers, err := h.circleRepo.GetCircleUsers(c, currentUser.CircleID)
	if err != nil {
		log.Errorw("Failed to retrieve circle users", "error", err)
		c.JSON(500, gin.H{"error": "Failed to retrieve circle users"})
		return
	}

	// Only the person who completed it, the assigned user, or admin can undo
	canUndo := false
	for _, cu := range circleUsers {
		if cu.UserID == currentUser.ID {
			if cu.UserID == history.CompletedBy ||
			   (history.AssignedTo != nil && cu.UserID == *history.AssignedTo) ||
			   cu.Role == "admin" {
				canUndo = true
				break
			}
		}
	}

	if !canUndo {
		log.Debugw("chore.api.UndoChoreCompletion user not authorized", "userID", currentUser.ID, "choreID", choreID)
		c.JSON(403, gin.H{"error": "Not authorized to undo this completion"})
		return
	}

	// Undo the completion
	restoredDueDate := req.NewDueDate
	if restoredDueDate == nil && history.DueDate != nil {
		restoredDueDate = history.DueDate
	}

	if err := h.choreRepo.UndoChoreCompletion(c, chore, history, restoredDueDate); err != nil {
		log.Errorw("chore.api.UndoChoreCompletion failed to undo", "error", err)
		c.JSON(500, gin.H{"error": "Failed to undo completion"})
		return
	}

	// Get updated chore
	updatedChore, err := h.choreRepo.GetChore(c, choreID, currentUser.ID)
	if err != nil {
		c.JSON(500, gin.H{"error": "Error getting updated chore"})
		return
	}

	c.JSON(200, gin.H{
		"message": "Completion undone successfully",
		"chore":   updatedChore,
	})
}

func (h *API) GetCircleMembers(c *gin.Context) {
	currentUser := auth.MustCurrentUser(c)
	users, err := h.circleRepo.GetCircleUsers(c, currentUser.CircleID)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get circle members"})
		return
	}
	if len(users) == 0 {
		c.JSON(404, gin.H{"error": "No members found in the circle"})
		return
	}
	c.JSON(200, users)
}
func (h *API) DeleteChore(c *gin.Context) {
	choreIDRaw := c.Param("id")
	choreID, err := strconv.Atoi(choreIDRaw)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid chore ID"})
		return
	}
	currentUser := auth.MustCurrentUser(c)
	chore, err := h.choreRepo.GetChore(c, choreID, currentUser.ID)
	if err != nil {
		c.JSON(404, gin.H{"error": "Chore not found"})
		return
	}

	// Get user's role in the circle
	circleUsers, err := h.circleRepo.GetCircleUsers(c, currentUser.CircleID)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get circle users"})
		return
	}

	canDelete := false
	for _, cu := range circleUsers {
		if cu.UserID == currentUser.ID {
			// Allow deletion if user is the creator, or is admin/manager
			if chore.CreatedBy == currentUser.ID || cu.Role == "admin" || cu.Role == "manager" {
				canDelete = true
				break
			}
		}
	}

	if !canDelete {
		c.JSON(403, gin.H{"error": "Only the creator or admins/managers can delete this chore"})
		return
	}
	if err := h.choreRepo.DeleteChore(c, choreID); err != nil {
		c.JSON(500, gin.H{"error": "Failed to delete chore"})
		return
	}
	c.JSON(200, gin.H{"message": "Chore deleted successfully"})
}

// GetChoresHistory returns chore completion history for External API
func (h *API) GetChoresHistory(c *gin.Context) {
	currentUser := auth.MustCurrentUser(c)

	// Parse query parameters
	daysStr := c.DefaultQuery("days", "7")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 {
		days = 7
	}

	maxRecordsStr := c.DefaultQuery("maxRecords", "100")
	maxRecords, err := strconv.Atoi(maxRecordsStr)
	if err != nil || maxRecords <= 0 {
		maxRecords = 100
	}

	offsetStr := c.DefaultQuery("offset", "0")
	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	membersStr := c.DefaultQuery("members", "false")
	includeMembers := membersStr == "true"

	// Get history from repository
	histories, err := h.choreRepo.GetChoresHistoryWithPagination(c, currentUser.ID, currentUser.CircleID, days, maxRecords, offset, includeMembers)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to fetch chore history"})
		return
	}

	// Check if there are more records
	hasMore := len(histories) == maxRecords

	c.JSON(200, gin.H{
		"res":     histories,
		"count":   len(histories),
		"hasMore": hasMore,
	})
}

// GetMonthlyPoints returns points leaderboard for External API
func (h *API) GetMonthlyPoints(c *gin.Context) {
	currentUser := auth.MustCurrentUser(c)

	// Parse period parameter
	period := c.DefaultQuery("period", "month")

	var since time.Time
	switch period {
	case "week":
		since = time.Now().AddDate(0, 0, -7)
	case "month":
		since = time.Now().AddDate(0, -1, 0)
	case "all":
		since = time.Time{} // Beginning of time
	default:
		since = time.Now().AddDate(0, -1, 0) // Default to month
		period = "month"
	}

	// Get all circle members
	members, err := h.circleRepo.GetCircleUsers(c, currentUser.CircleID)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get circle members"})
		return
	}

	type PointsResult struct {
		UserID   int    `json:"userId"`
		UserName string `json:"userName"`
		Points   int    `json:"points"`
		Rank     int    `json:"rank"`
	}

	// Calculate points for each member
	var results []PointsResult
	for _, member := range members {
		var totalPoints int
		err := h.choreRepo.GetDB().WithContext(c).
			Table("chore_histories").
			Select("COALESCE(SUM(chore_histories.points), 0) as total_points").
			Joins("LEFT JOIN chores ON chore_histories.chore_id = chores.id").
			Where("chore_histories.completed_by = ? AND chores.circle_id = ?", member.UserID, currentUser.CircleID).
			Where("chore_histories.performed_at > ?", since).
			Scan(&totalPoints).Error

		if err != nil {
			continue // Skip on error
		}

		results = append(results, PointsResult{
			UserID:   member.UserID,
			UserName: member.DisplayName,
			Points:   totalPoints,
		})
	}

	// Sort by points descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Points > results[j].Points
	})

	// Assign ranks
	for i := range results {
		results[i].Rank = i + 1
	}

	c.JSON(200, gin.H{
		"res":    results,
		"period": period,
	})
}

// executeThingAction executes the configured action on a Thing when a chore is completed
func (h *API) executeThingAction(c *gin.Context, chore *chModel.Chore) error {
	log := logging.FromContext(c)

	if chore.ThingChore == nil || chore.ThingChore.ThingID <= 0 || chore.ThingChore.ActionType == "" {
		return nil
	}

	// Fetch the Thing
	thing, err := h.tRepo.GetThingByID(c, chore.ThingChore.ThingID)
	if err != nil {
		return err
	}

	oldState := thing.State
	newState := oldState
	actionExecuted := false

	// Execute action based on type
	switch chore.ThingChore.ActionType {
	case "set":
		if chore.ThingChore.ActionValue != "" {
			newState = chore.ThingChore.ActionValue
			actionExecuted = true
		}

	case "toggle":
		if thing.Type == "boolean" {
			if oldState == "false" || oldState == "" {
				newState = "true"
			} else {
				newState = "false"
			}
			actionExecuted = true
		}

	case "increment":
		if thing.Type == "number" {
			currentValue, err := strconv.Atoi(oldState)
			if err == nil {
				incrementBy := 1
				if chore.ThingChore.ActionValue != "" {
					if val, err := strconv.Atoi(chore.ThingChore.ActionValue); err == nil {
						incrementBy = val
					}
				}
				newState = strconv.Itoa(currentValue + incrementBy)
				actionExecuted = true
			}
		}

	case "decrement":
		if thing.Type == "number" {
			currentValue, err := strconv.Atoi(oldState)
			if err == nil {
				decrementBy := 1
				if chore.ThingChore.ActionValue != "" {
					if val, err := strconv.Atoi(chore.ThingChore.ActionValue); err == nil {
						decrementBy = val
					}
				}
				newState = strconv.Itoa(currentValue - decrementBy)
				actionExecuted = true
			}
		}
	}

	// Update the Thing if action was executed
	if actionExecuted && newState != oldState {
		thing.State = newState
		if err := h.tRepo.UpdateThingState(c, thing); err != nil {
			return err
		}
		log.Infow("Thing action executed",
			"thingId", thing.ID,
			"choreId", chore.ID,
			"actionType", chore.ThingChore.ActionType,
			"oldState", oldState,
			"newState", newState)

		// After Thing state changes, evaluate triggers and schedule affected chores
		if err := h.evaluateTriggersAfterThingChange(c, thing); err != nil {
			log.Warnw("Failed to evaluate triggers after Thing change",
				"thingId", thing.ID,
				"error", err)
			// Don't fail the whole operation, just log the warning
		}
	}

	return nil
}

// evaluateTriggersAfterThingChange evaluates all chores triggered by a Thing state change
// and schedules them according to their frequency settings
func (h *API) evaluateTriggersAfterThingChange(c *gin.Context, thing *tModel.Thing) error {
	// Get all thing_chores for this Thing
	thingChores, err := h.tRepo.GetThingChoresByThingId(c, thing.ID)
	if err != nil {
		return err
	}

	triggerTime := time.Now().UTC()

	for _, tc := range thingChores {
		// Check if this chore's trigger matches the current Thing state
		triggered := isThingTriggerSatisfied(thing.State, tc.TriggerState, tc.Condition)

		if triggered {
			// Get the full chore to access frequency settings
			choreObj, err := h.choreRepo.GetChore(c, tc.ChoreID, thing.UserID)
			if err != nil {
				return err
			}

			// Calculate next due date using frequency settings
			var dueDate time.Time
			scheduledDate := ScheduleTriggeredChore(choreObj, triggerTime)
			if scheduledDate != nil {
				dueDate = *scheduledDate
			} else {
				// No frequency configured, set to trigger time
				dueDate = triggerTime
			}

			// Set due date if expired or doesn't exist
			if err := h.choreRepo.SetDueDateIfExpired(c, tc.ChoreID, dueDate); err != nil {
				return err
			}
		}
	}

	return nil
}

func APIs(cfg *config.Config, api *API, r *gin.Engine, auth *jwt.GinJWTMiddleware, limiter *limiter.Limiter, userRepo *uRepo.UserRepository) {

	tasksAPI := r.Group("eapi/v1/chore")
	tasksAPI.Use(
		utils.TimeoutMiddleware(cfg.Server.WriteTimeout),
		utils.RateLimitMiddleware(limiter),
		authMiddleware.APITokenMiddleware(userRepo),
	)
	{
		tasksAPI.GET("", api.GetAllChores)
		tasksAPI.GET("/:id", api.GetChore)
		tasksAPI.GET("/archived", api.GetArchivedChores)
		tasksAPI.POST("", api.CreateChore)
		tasksAPI.DELETE("/:id", api.DeleteChore)
		tasksAPI.GET("/history", api.GetChoresHistory)
		tasksAPI.GET("/points", api.GetMonthlyPoints)
	}

	// Plus member only endpoints
	tasksPlusAPI := r.Group("eapi/v1/chore")
	tasksPlusAPI.Use(
		utils.TimeoutMiddleware(cfg.Server.WriteTimeout),
		utils.RateLimitMiddleware(limiter),
		authMiddleware.APITokenMiddleware(userRepo),
		authMiddleware.RequirePlusMemberMiddleware(),
	)
	{
		tasksPlusAPI.POST("/:id/complete", api.CompleteChore)
		tasksPlusAPI.PUT("/:id", api.UpdateChore)
	}

	circleAPI := r.Group("eapi/v1/circle")
	circleAPI.Use(
		utils.TimeoutMiddleware(cfg.Server.WriteTimeout),
		utils.RateLimitMiddleware(limiter),
		authMiddleware.APITokenMiddleware(userRepo),
		authMiddleware.RequirePlusMemberMiddleware(),
	)
	{
		circleAPI.GET("/members", api.GetCircleMembers)
	}

}
