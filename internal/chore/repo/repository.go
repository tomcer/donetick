package chore

import (
	"context"
	"errors"
	"fmt"
	"time"

	config "donetick.com/core/config"
	chModel "donetick.com/core/internal/chore/model"
	cModel "donetick.com/core/internal/circle/model"
	storageModel "donetick.com/core/internal/storage/model"
	stModel "donetick.com/core/internal/subtask/model"
	"donetick.com/core/logging"
	"gorm.io/gorm"
)

type ChoreRepository struct {
	db     *gorm.DB
	dbType string
}

func NewChoreRepository(db *gorm.DB, cfg *config.Config) *ChoreRepository {
	return &ChoreRepository{db: db, dbType: cfg.Database.Type}
}

// GetDB returns the underlying database connection for complex queries
func (r *ChoreRepository) GetDB() *gorm.DB {
	return r.db
}

func (r *ChoreRepository) UpsertChore(c context.Context, chore *chModel.Chore) error {
	return r.db.WithContext(c).Model(&chore).Save(chore).Error
}
func (r *ChoreRepository) UpdateChorePriority(c context.Context, userID int, choreID int, priority int) error {
	var affectedRows int64
	r.db.WithContext(c).Model(&chModel.Chore{}).Where("id = ?", choreID).Update("priority", priority).Count(&affectedRows)
	if affectedRows == 0 {
		return errors.New("no rows affected")
	}
	return nil
}

func (r *ChoreRepository) UpdateChoreFields(ctx context.Context, choreID int, fields map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&chModel.Chore{}).Where("id = ?", choreID).Updates(fields).Error
}

func (r *ChoreRepository) UpdateChores(c context.Context, chores []*chModel.Chore) error {
	return r.db.WithContext(c).Save(&chores).Error
}
func (r *ChoreRepository) CreateChore(c context.Context, chore *chModel.Chore) (int, error) {
	if err := r.db.WithContext(c).Create(chore).Error; err != nil {
		return 0, err
	}
	return chore.ID, nil
}

func (r *ChoreRepository) GetChore(c context.Context, choreID int, userID int) (*chModel.Chore, error) {
	var chore chModel.Chore
	query := r.db.WithContext(c).Model(&chModel.Chore{}).
		Preload("SubTasks", "chore_id = ?", choreID).
		Preload("Assignees").
		Preload("ThingChore").
		Preload("LabelsV2").
		Joins("LEFT JOIN chore_assignees ON chores.id = chore_assignees.chore_id AND chore_assignees.user_id = ?", userID).
		Where("chores.id = ? AND ((chores.is_private = false) OR (chores.is_private = true AND (chores.created_by = ? OR chore_assignees.user_id = ?)))", choreID, userID, userID)

	if err := query.First(&chore).Error; err != nil {
		return nil, err
	}
	return &chore, nil
}

func (r *ChoreRepository) GetChores(c context.Context, circleID int, userID int, includeArchived bool) ([]*chModel.Chore, error) {
	var chores []*chModel.Chore
	query := r.db.WithContext(c).Preload("Assignees").Preload("LabelsV2").Preload("ThingChore").Joins("left join chore_assignees on chores.id = chore_assignees.chore_id").Where("chores.circle_id = ? AND ((chores.is_private = false) OR (chores.is_private = true AND (chores.created_by = ? OR chore_assignees.user_id = ?)))", circleID, userID, userID).Group("chores.id").Order("next_due_date asc")
	if !includeArchived {
		query = query.Where("chores.is_active = ?", true)
	}
	if err := query.Find(&chores, "circle_id = ?", circleID).Error; err != nil {
		return nil, err
	}
	return chores, nil
}

func (r *ChoreRepository) GetArchivedChores(c context.Context, circleID int, userID int) ([]*chModel.Chore, error) {
	var chores []*chModel.Chore
	if err := r.db.WithContext(c).Preload("Assignees").Preload("LabelsV2").Joins("left join chore_assignees on chores.id = chore_assignees.chore_id").Where("chores.circle_id = ? AND ((chores.is_private = false) OR (chores.is_private = true AND (chores.created_by = ? OR chore_assignees.user_id = ?)))", circleID, userID, userID).Group("chores.id").Order("next_due_date asc").Find(&chores, "circle_id = ? AND is_active = ?", circleID, false).Error; err != nil {
		return nil, err
	}
	return chores, nil
}

func (r *ChoreRepository) DeleteChore(c context.Context, id int) error {
	return r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("chore_id = ?", id).Delete(&chModel.ChoreAssignees{}).Error; err != nil {
			return err
		}
		if err := tx.Where("chore_id = ?", id).Delete(&chModel.TimeSession{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&chModel.ChoreHistory{}, "chore_id = ?", id).Error; err != nil {
			return err
		}
		// subtask if exists:
		if err := tx.Where("chore_id = ?", id).Delete(&stModel.SubTask{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&chModel.Chore{}, id).Error; err != nil {
			return err
		}
		// Delete all subtasks associated with the chore
		if err := tx.Where("chore_id = ?", id).Delete(&stModel.SubTask{}).Error; err != nil {
			return err
		}
		// Delete all chore storage files associated with the chore:
		if err := tx.Where("entity_type = ? AND entity_id = ?", storageModel.EntityTypeChoreDescription, id).Delete(&storageModel.StorageFile{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func (r *ChoreRepository) SoftDelete(c context.Context, id int, userID int) error {
	return r.db.WithContext(c).Model(&chModel.Chore{}).Where("id = ?", id).Where("created_by = ? ", userID).Update("is_active", false).Error

}

func (r *ChoreRepository) IsChoreOwner(c context.Context, choreID int, userID int) error {
	var chore chModel.Chore
	err := r.db.WithContext(c).Model(&chModel.Chore{}).Where("id = ? AND created_by = ?", choreID, userID).First(&chore).Error
	return err
}

func (r *ChoreRepository) SetChorePendingApproval(c context.Context, chore *chModel.Chore, note *string, userID int, completedDate *time.Time) error {
	err := r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		// Look for existing chore history with start or pause status
		var existingHistory chModel.ChoreHistory
		err := tx.Where("chore_id = ? AND status = ? ",
			chore.ID, chModel.ChoreHistoryStatusStarted).
			First(&existingHistory).Error

		var ch *chModel.ChoreHistory
		if err == nil {
			// Update existing history record to mark as pending approval
			existingHistory.PerformedAt = completedDate
			existingHistory.Note = note
			existingHistory.Status = chModel.ChoreHistoryStatusPendingApproval
			ch = &existingHistory
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create a new chore history record
			ch = &chModel.ChoreHistory{
				ChoreID:     chore.ID,
				PerformedAt: completedDate,
				CompletedBy: userID,
				AssignedTo:  chore.AssignedTo,
				DueDate:     chore.NextDueDate,
				Note:        note,
				Status:      chModel.ChoreHistoryStatusPendingApproval,
			}
		} else {
			return err
		}

		if err := tx.Save(ch).Error; err != nil {
			return err
		}

		// Set chore status to pending approval
		if err := tx.Model(&chModel.Chore{}).Where("id = ?", chore.ID).Update("status", chModel.ChoreStatusPendingApproval).Error; err != nil {
			return err
		}

		// if there is any time session associated with the chore, mark them as finished:
		var timeSessions []*chModel.TimeSession
		tx.Model(&chModel.TimeSession{}).Where("chore_id = ? AND status < ?", chore.ID, chModel.TimeSessionStatusCompleted).Find(&timeSessions)
		if len(timeSessions) != 0 {
			for _, session := range timeSessions {
				session.Finish(userID)
			}
			if err := tx.Save(&timeSessions).Error; err != nil {
				return err
			}
		}

		return nil
	})
	return err
}

func (r *ChoreRepository) ApproveChore(c context.Context, chore *chModel.Chore, adminUserID int, dueDate *time.Time, nextAssignedTo int, applyPoints bool) error {
	err := r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		choreUpdates := map[string]interface{}{}
		choreUpdates["next_due_date"] = dueDate
		choreUpdates["status"] = chModel.ChoreStatusNoStatus

		// PATCH 2: Always save rotation, don't archive trigger chores
		choreUpdates["assigned_to"] = nextAssignedTo

		// Archive only non-trigger one-time tasks
		if dueDate == nil && chore.FrequencyType != "trigger" {
			choreUpdates["is_active"] = false
		}

		// Get the latest history entry that's pending approval and update it to completed
		var history chModel.ChoreHistory
		err := tx.Where("chore_id = ? AND status = ?", chore.ID, chModel.ChoreHistoryStatusPendingApproval).
			Order("performed_at desc").
			First(&history).Error
		if err != nil {
			return err
		}

		// Update status to completed
		history.Status = chModel.ChoreHistoryStatusCompleted

		// Update UserCircle Points if applicable
		if applyPoints && chore.Points != nil && *chore.Points > 0 {
			history.Points = chore.Points
			if err := tx.Model(&cModel.UserCircle{}).Where("user_id = ? AND circle_id = ?", history.CompletedBy, chore.CircleID).Update("points", gorm.Expr("points + ?", chore.Points)).Error; err != nil {
				return err
			}
		}

		// Save the updated history
		if err := tx.Save(&history).Error; err != nil {
			return err
		}

		// Perform the update operation once, using the prepared updates map.
		if err := tx.Model(&chModel.Chore{}).Where("id = ?", chore.ID).Updates(choreUpdates).Error; err != nil {
			return err
		}

		return nil
	})
	return err
}

func (r *ChoreRepository) RejectChore(c context.Context, choreID int, rejectionNote *string) error {
	return r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		// Reset chore status to normal
		if err := tx.Model(&chModel.Chore{}).Where("id = ?", choreID).Update("status", chModel.ChoreStatusNoStatus).Error; err != nil {
			return err
		}

		// Get the latest pending approval history entry and mark it as rejected
		var history chModel.ChoreHistory
		err := tx.Where("chore_id = ? AND status = ?", choreID, chModel.ChoreHistoryStatusPendingApproval).
			Order("performed_at desc").
			First(&history).Error
		if err != nil {
			return err
		}

		// Update status to rejected
		history.Status = chModel.ChoreHistoryStatusRejected

		// Save the updated history
		if err := tx.Save(&history).Error; err != nil {
			return err
		}

		return nil
	})
}

func (r *ChoreRepository) CompleteChore(c context.Context, chore *chModel.Chore, note *string, userID int, dueDate *time.Time, completedDate *time.Time, nextAssignedTo int, applyPoints bool, customPoints *int) error {
	err := r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {

		choreUpdates := map[string]interface{}{}
		choreUpdates["next_due_date"] = dueDate
		choreUpdates["status"] = chModel.ChoreStatusNoStatus

		// PATCH 2: Always save rotation, don't archive trigger chores
		choreUpdates["assigned_to"] = nextAssignedTo

		// Archive only non-trigger one-time tasks
		if dueDate == nil && chore.FrequencyType != "trigger" {
			choreUpdates["is_active"] = false
		}

		// Look for existing chore history with start or pause status
		var existingHistory chModel.ChoreHistory
		err := tx.Where("chore_id = ? AND status = ? ",
			chore.ID, chModel.ChoreHistoryStatusStarted).
			First(&existingHistory).Error

		var ch *chModel.ChoreHistory
		if err == nil {
			// Update existing history record
			existingHistory.PerformedAt = completedDate
			existingHistory.Note = note
			existingHistory.Status = chModel.ChoreHistoryStatusCompleted
			ch = &existingHistory
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create a new chore history record
			ch = &chModel.ChoreHistory{
				ChoreID:     chore.ID,
				PerformedAt: completedDate,
				CompletedBy: userID,
				AssignedTo:  chore.AssignedTo,
				DueDate:     chore.NextDueDate,
				Note:        note,
				Status:      chModel.ChoreHistoryStatusCompleted,
			}
		} else {
			return err
		}

		// Update UserCircle Points :
		// Use customPoints if provided, otherwise use chore.Points
		pointsToAward := chore.Points
		if customPoints != nil {
			pointsToAward = customPoints
		}

		if applyPoints && pointsToAward != nil && *pointsToAward > 0 {
			ch.Points = pointsToAward
			if err := tx.Model(&cModel.UserCircle{}).Where("user_id = ? AND circle_id = ?", userID, chore.CircleID).Update("points", gorm.Expr("points + ?", pointsToAward)).Error; err != nil {
				return err
			}
		}
		// Perform the update operation once, using the prepared updates map.
		if err := tx.Model(&chModel.Chore{}).Where("id = ?", chore.ID).Updates(choreUpdates).Error; err != nil {
			return err
		}

		if err := tx.Save(ch).Error; err != nil {
			return err
		}
		// if there is any time session associated with the chore, mark them as finished:
		var timeSessions []*chModel.TimeSession
		tx.Model(&chModel.TimeSession{}).Where("chore_id = ? AND status < ?", chore.ID, chModel.TimeSessionStatusCompleted).Find(&timeSessions)
		if len(timeSessions) != 0 {
			for _, session := range timeSessions {
				session.Finish(userID)
			}
			if err := tx.Save(&timeSessions).Error; err != nil {
				return err
			}
		}

		return nil
	})
	return err
}

func (r *ChoreRepository) SkipChore(c context.Context, chore *chModel.Chore, userID int, dueDate *time.Time, nextAssignedTo *int) error {
	err := r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		choreUpdates := map[string]interface{}{}
		choreUpdates["next_due_date"] = dueDate
		choreUpdates["status"] = chModel.ChoreStatusNoStatus

		// PATCH 2: Always save rotation, don't archive trigger chores
		choreUpdates["assigned_to"] = nextAssignedTo

		// Archive only non-trigger one-time tasks
		if dueDate == nil && chore.FrequencyType != "trigger" {
			choreUpdates["is_active"] = false
		}

		// Look for existing chore history with start or pause status
		var existingHistory chModel.ChoreHistory
		err := tx.Where("chore_id = ? AND status = ? ",
			chore.ID, chModel.ChoreHistoryStatusStarted).
			First(&existingHistory).Error

		var ch *chModel.ChoreHistory
		skippedAt := time.Now().UTC()

		if err == nil && existingHistory.PerformedAt != nil {
			// Update existing history record
			existingHistory.PerformedAt = &skippedAt
			existingHistory.Note = nil
			existingHistory.Status = chModel.ChoreHistoryStatusSkipped
			ch = &existingHistory
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create a new chore history record for the skipped chore
			ch = &chModel.ChoreHistory{
				ChoreID:     chore.ID,
				PerformedAt: &skippedAt,
				CompletedBy: userID,
				AssignedTo:  chore.AssignedTo,
				DueDate:     chore.NextDueDate,
				Note:        nil,
				Status:      chModel.ChoreHistoryStatusSkipped,
			}
		} else {
			return err
		}

		// Perform the update operation once, using the prepared updates map.
		if err := tx.Model(&chModel.Chore{}).Where("id = ?", chore.ID).Updates(choreUpdates).Error; err != nil {
			return err
		}

		if err := tx.Save(ch).Error; err != nil {
			return err
		}

		// if there is any time session associated with the chore, mark them as finished:
		var timeSessions []*chModel.TimeSession
		tx.Model(&chModel.TimeSession{}).Where("chore_id = ? AND status < ?", chore.ID, chModel.TimeSessionStatusCompleted).Find(&timeSessions)
		if len(timeSessions) != 0 {
			for _, session := range timeSessions {
				session.Finish(userID)
			}
			if err := tx.Save(&timeSessions).Error; err != nil {
				return err
			}
		}

		return nil
	})
	return err
}

func (r *ChoreRepository) GetChoreHistory(c context.Context, choreID int) ([]*chModel.ChoreHistory, error) {
	var histories []*chModel.ChoreHistory
	if err := r.db.WithContext(c).
		Table("chore_histories").
		Select("chore_histories.*, time_sessions.duration").
		Joins("LEFT JOIN time_sessions ON chore_histories.id = time_sessions.chore_history_id").
		Where("chore_histories.chore_id = ?", choreID).
		Order("chore_histories.updated_at desc").
		Find(&histories).Error; err != nil {
		return nil, err
	}
	return histories, nil
}
func (r *ChoreRepository) GetChoreHistoryWithLimit(c context.Context, choreID int, limit int) ([]*chModel.ChoreHistory, error) {
	var histories []*chModel.ChoreHistory
	if err := r.db.WithContext(c).
		Table("chore_histories").
		Select("chore_histories.*, time_sessions.duration").
		Joins("LEFT JOIN time_sessions ON chore_histories.id = time_sessions.chore_history_id").
		Where("chore_histories.chore_id = ?", choreID).
		Order("chore_histories.performed_at desc").
		Limit(limit).
		Find(&histories).Error; err != nil {
		return nil, err
	}
	return histories, nil
}

func (r *ChoreRepository) GetChoreHistoryByID(c context.Context, choreID int, historyID int) (*chModel.ChoreHistory, error) {
	var history chModel.ChoreHistory
	if err := r.db.WithContext(c).Where("id = ? and chore_id = ? ", historyID, choreID).First(&history).Error; err != nil {
		return nil, err
	}
	return &history, nil
}

func (r *ChoreRepository) UpdateChoreHistory(c context.Context, history *chModel.ChoreHistory) error {
	return r.db.WithContext(c).Save(history).Error
}

func (r *ChoreRepository) UpdateLatestChoreHistory(c context.Context, choreID int, updates map[string]interface{}) error {
	//get the latest chore history for the given chore ID
	var latestHistory chModel.ChoreHistory
	if err := r.db.WithContext(c).Where("chore_id = ?", choreID).Order("created_at desc").First(&latestHistory).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("no history found for chore ID %d", choreID)
		}
		return err
	}
	// Update the latest history with the provided updates
	if err := r.db.WithContext(c).Model(&latestHistory).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update latest chore history: %w", err)
	}
	return nil
}

func (r *ChoreRepository) GetLatestChoreHistory(c context.Context, choreID int) (*chModel.ChoreHistory, error) {
	var history chModel.ChoreHistory
	err := r.db.WithContext(c).
		Where("chore_id = ? AND status = ?", choreID, chModel.ChoreHistoryStatusCompleted).
		Order("id desc").
		First(&history).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("no completed history found for chore ID %d", choreID)
		}
		return nil, err
	}
	return &history, nil
}

func (r *ChoreRepository) UndoChoreCompletion(c context.Context, chore *chModel.Chore, history *chModel.ChoreHistory, newDueDate *time.Time) error {
	return r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		// 1. Delete the history entry
		if err := tx.Delete(history).Error; err != nil {
			return fmt.Errorf("failed to delete history entry: %w", err)
		}

		// 2. Deduct points from the user if points were awarded
		if history.Points != nil && *history.Points > 0 {
			if err := tx.Model(&cModel.UserCircle{}).
				Where("user_id = ? AND circle_id = ?", history.CompletedBy, chore.CircleID).
				Update("points", gorm.Expr("points - ?", *history.Points)).Error; err != nil {
				return fmt.Errorf("failed to deduct points: %w", err)
			}
		}

		// 3. Restore chore to previous state
		choreUpdates := map[string]interface{}{
			"is_active": true, // Unarchive if it was archived
			"status":    chModel.ChoreStatusNoStatus,
		}

		// Restore assignee from history
		if history.AssignedTo != nil {
			choreUpdates["assigned_to"] = *history.AssignedTo
		}

		// Set due date: use provided newDueDate, or restore from history
		if newDueDate != nil {
			choreUpdates["next_due_date"] = newDueDate
		} else if history.DueDate != nil {
			choreUpdates["next_due_date"] = history.DueDate
		}

		if err := tx.Model(&chModel.Chore{}).Where("id = ?", chore.ID).Updates(choreUpdates).Error; err != nil {
			return fmt.Errorf("failed to restore chore state: %w", err)
		}

		// 4. Reopen time sessions that were closed with this completion
		// Find any time sessions that were finished at the same time as this completion
		if history.PerformedAt != nil {
			var timeSessions []*chModel.TimeSession
			// Find sessions that were completed around the same time (within 5 seconds)
			timeWindow := history.PerformedAt.Add(-5 * time.Second)
			if err := tx.Model(&chModel.TimeSession{}).
				Where("chore_id = ? AND status = ? AND updated_at >= ?",
					chore.ID, chModel.TimeSessionStatusCompleted, timeWindow).
				Find(&timeSessions).Error; err == nil && len(timeSessions) > 0 {

				// Reopen the most recent session
				for _, session := range timeSessions {
					session.Status = chModel.TimeSessionStatusPaused
					session.EndTime = nil
				}
				if err := tx.Save(&timeSessions).Error; err != nil {
					// Don't fail the undo if time session reopening fails, just log it
					// The history deletion and point deduction are more critical
				}
			}
		}

		return nil
	})
}

func (r *ChoreRepository) DeleteChoreHistory(c context.Context, historyID int) error {
	// create transaction and delete all the chore timer assiociated with the chore history then delete the chore history
	return r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(c).Where("chore_history_id = ?", historyID).Delete(&chModel.TimeSession{}).Error; err != nil {
			return fmt.Errorf("failed to delete chore time sessions: %w", err)
		}
		// Now delete the chore history
		if err := tx.WithContext(c).Delete(&chModel.ChoreHistory{}, historyID).Error; err != nil {
			return fmt.Errorf("failed to delete chore history: %w", err)
		}
		return nil
	})

}

func (r *ChoreRepository) UpdateChoreAssignees(c context.Context, assignees []*chModel.ChoreAssignees) error {
	return r.db.WithContext(c).Save(&assignees).Error
}

func (r *ChoreRepository) DeleteChoreAssignees(c context.Context, choreAssignees []*chModel.ChoreAssignees) error {
	return r.db.WithContext(c).Delete(&choreAssignees).Error
}

func (r *ChoreRepository) GetChoreAssignees(c context.Context, choreID int) ([]*chModel.ChoreAssignees, error) {
	var assignees []*chModel.ChoreAssignees
	if err := r.db.WithContext(c).Find(&assignees, "chore_id = ?", choreID).Error; err != nil {
		return nil, err
	}
	return assignees, nil
}

func (r *ChoreRepository) RemoveChoreAssigneeByCircleID(c context.Context, userID int, circleID int) error {
	return r.db.WithContext(c).Where("user_id = ? AND chore_id IN (SELECT id FROM chores WHERE circle_id = ? and created_by != ?)", userID, circleID, userID).Delete(&chModel.ChoreAssignees{}).Error
}

// func (r *ChoreReposity) GetOverdueChoresForNotification(c context.Context, overdueDuration time.Duration, everyDuration time.Duration, untilDuration time.Duration) ([]*chModel.Chore, error) {
// 	var chores []*chModel.Chore
// 	query := r.db.Debug().WithContext(c).Table("chores").Select("chores.*, MAX(n.created_at) as max_notification_created_at").Joins("left join notifications n on n.chore_id = chores.id and n.scheduled_for = chores.next_due_date and n.type = 2")
// 	if err := query.Where("chores.is_active = ? and chores.notification = ? and chores.next_due_date < ? and chores.next_due_date > ?", true, true, time.Now().Add(overdueDuration).UTC(), time.Now().Add(untilDuration).UTC()).Where(readJSONBooleanField(r.dbType, "chores.notification_meta", "nagging")).Having("MAX(n.created_at) is null or MAX(n.created_at) < ?", time.Now().Add(everyDuration).UTC()).Group("chores.id").Find(&chores).Error; err != nil {
// 		return nil, err
// 	}
// 	return chores, nil
// }

func (r *ChoreRepository) GetOverdueChoresForNotification(c context.Context, overdueFor time.Duration, everyDuration time.Duration, untilDuration time.Duration) ([]*chModel.Chore, error) {
	var chores []*chModel.Chore
	now := time.Now().UTC()
	overdueTime := now.Add(-overdueFor)
	everyTime := now.Add(-everyDuration)
	untilTime := now.Add(-untilDuration)

	query := r.db.Debug().WithContext(c).
		Table("chores").
		Select("chores.*, MAX(n.created_at) as max_notification_created_at").
		Joins("left join notifications n on n.chore_id = chores.id and n.type = 2").
		Where("chores.is_active = ? AND chores.notification = ? AND chores.next_due_date < ? AND chores.next_due_date > ?", true, true, overdueTime, untilTime).
		Where(readJSONBooleanField(r.dbType, "chores.notification_meta", "nagging")).
		Group("chores.id").
		Having("MAX(n.created_at) IS NULL OR MAX(n.created_at) < ?", everyTime)

	if err := query.Find(&chores).Error; err != nil {
		return nil, err
	}

	return chores, nil
}

// a predue notfication is a notification send before the due date in 6 hours, 3 hours :
func (r *ChoreRepository) GetPreDueChoresForNotification(c context.Context, preDueDuration time.Duration, everyDuration time.Duration) ([]*chModel.Chore, error) {
	var chores []*chModel.Chore
	query := r.db.WithContext(c).Table("chores").Select("chores.*, MAX(n.created_at) as max_notification_created_at").Joins("left join notifications n on n.chore_id = chores.id and n.scheduled_for = chores.next_due_date and n.type = 3")
	if err := query.Where("chores.is_active = ? and chores.notification = ? and chores.next_due_date > ? and chores.next_due_date < ?", true, true, time.Now().UTC(), time.Now().Add(everyDuration*2).UTC()).Where(readJSONBooleanField(r.dbType, "chores.notification_meta", "predue")).Having("MAX(n.created_at) is null or MAX(n.created_at) < ?", time.Now().Add(everyDuration).UTC()).Group("chores.id").Find(&chores).Error; err != nil {
		return nil, err
	}
	return chores, nil
}

func readJSONBooleanField(dbType string, columnName string, fieldName string) string {
	if dbType == "postgres" {
		return fmt.Sprintf("(%s::json->>'%s')::boolean", columnName, fieldName)
	}
	return fmt.Sprintf("JSON_EXTRACT(%s, '$.%s')", columnName, fieldName)
}

func (r *ChoreRepository) SetDueDate(c context.Context, choreID int, dueDate time.Time) error {
	return r.db.WithContext(c).Model(&chModel.Chore{}).Where("id = ?", choreID).Updates(map[string]interface{}{
		"next_due_date": dueDate,
		"is_active":     true,
	}).Error
}

func (r *ChoreRepository) SetDueDateIfNotExisted(c context.Context, choreID int, dueDate time.Time) error {
	return r.db.WithContext(c).Model(&chModel.Chore{}).Where("id = ? and next_due_date is null", choreID).Updates(map[string]interface{}{
		"next_due_date": dueDate,
		"is_active":     true,
	}).Error
}

// SetDueDateIfExpired sets the due date if it's null or expired (< now)
func (r *ChoreRepository) SetDueDateIfExpired(c context.Context, choreID int, dueDate time.Time) error {
	now := time.Now().UTC()
	return r.db.WithContext(c).Model(&chModel.Chore{}).
		Where("id = ? AND (next_due_date IS NULL OR next_due_date < ?)", choreID, now).
		Updates(map[string]interface{}{
			"next_due_date": dueDate,
			"is_active":     true,
		}).Error
}

func (r *ChoreRepository) GetChoreDetailByID(c context.Context, choreID int, circleID int, userID int) (*chModel.ChoreDetail, error) {
	var choreDetail chModel.ChoreDetail
	if err := r.db.WithContext(c).
		Table("chores").
		Preload("Subtasks").
		Select(`
        chores.id, 
        chores.name,
		chores.description, 
        chores.frequency_type, 
        chores.next_due_date, 
        chores.assigned_to,
        chores.created_by,
		chores.priority,
		chores.completion_window,
		chores.status,
		CAST(MAX(time_sessions.duration) AS INTEGER) as duration,
		time_sessions.start_time as start_time,
		time_sessions.updated_at as timer_updated_at,
        recent_history.last_completed_date,
		recent_history.last_assigned_to,
		recent_history.notes,
        recent_history.last_completed_by as last_completed_by,
        COUNT(chore_histories.id) as total_completed`).
		Joins("LEFT JOIN chore_histories ON chores.id = chore_histories.chore_id").
		Joins(`LEFT JOIN (
        SELECT 
            chore_id, 
            assigned_to AS last_assigned_to, 
            performed_at AS last_completed_date,
			completed_by AS last_completed_by,
			notes
			
        FROM chore_histories
        WHERE (chore_id, performed_at) IN (
            SELECT chore_id, MAX(performed_at)
            FROM chore_histories
            GROUP BY chore_id
        )
    ) AS recent_history ON chores.id = recent_history.chore_id`).
		Joins("LEFT JOIN time_sessions ON chores.id = time_sessions.chore_id AND time_sessions.status < ?", chModel.TimeSessionStatusCompleted).
		Joins("LEFT JOIN chore_assignees ON chores.id = chore_assignees.chore_id AND chore_assignees.user_id = ?", userID).
		Where("chores.id = ? AND chores.circle_id = ? AND ((chores.is_private = false) OR (chores.is_private = true AND (chores.created_by = ? OR chore_assignees.user_id = ?)))", choreID, circleID, userID, userID).
		Group("chores.id, recent_history.last_completed_date, recent_history.last_assigned_to, recent_history.last_completed_by, recent_history.notes, time_sessions.start_time, time_sessions.updated_at").
		First(&choreDetail).Error; err != nil {
		return nil, err

	}
	return &choreDetail, nil
}

func (r *ChoreRepository) ArchiveChore(c context.Context, choreID int, userID int) error {
	return r.db.WithContext(c).Model(&chModel.Chore{}).Where("id = ? and created_by = ?", choreID, userID).Update("is_active", false).Error
}

func (r *ChoreRepository) UnarchiveChore(c context.Context, choreID int, userID int) error {
	return r.db.WithContext(c).Model(&chModel.Chore{}).Where("id = ? and created_by = ?", choreID, userID).Update("is_active", true).Error
}

func (r *ChoreRepository) GetChoresHistoryByUserID(c context.Context, userID int, circleID int, days int, includeCircle bool) ([]*chModel.ChoreHistory, error) {

	var chores []*chModel.ChoreHistory
	since := time.Now().AddDate(0, 0, days*-1)
	query := r.db.WithContext(c).
		Table("chore_histories").
		Select("chore_histories.*, circles.id as circle_id, time_sessions.duration, time_sessions.start_time, time_sessions.updated_at as timer_updated_at").
		Joins("LEFT JOIN chores ON chore_histories.chore_id = chores.id").
		Joins("LEFT JOIN circles ON chores.circle_id = circles.id").
		Joins("LEFT JOIN time_sessions ON chore_histories.id = time_sessions.chore_history_id").
		Joins("LEFT JOIN chore_assignees ON chores.id = chore_assignees.chore_id AND chore_assignees.user_id = ?", userID).
		Where("circles.id = ? AND chore_histories.updated_at > ?", circleID, since).
		Where("(chores.is_private = false) OR (chores.is_private = true AND (chores.created_by = ? OR chore_assignees.user_id = ?))", userID, userID).
		Order("chore_histories.performed_at desc, chore_histories.updated_at desc")

	if !includeCircle {
		query = query.Where("chore_histories.completed_by = ?", userID)
	}

	if err := query.Find(&chores).Error; err != nil {
		return nil, err
	}
	return chores, nil
}

// GetChoresHistoryWithPagination returns chore history with limit and offset support for External API
func (r *ChoreRepository) GetChoresHistoryWithPagination(c context.Context, userID int, circleID int, days int, maxRecords int, offset int, includeCircle bool) ([]*chModel.ChoreHistory, error) {
	var chores []*chModel.ChoreHistory
	since := time.Now().AddDate(0, 0, days*-1)

	// Apply safety limits
	if maxRecords <= 0 || maxRecords > 500 {
		maxRecords = 100 // Default
	}
	if days > 90 {
		days = 90 // Max 90 days
	}
	if offset < 0 {
		offset = 0
	}

	query := r.db.WithContext(c).
		Table("chore_histories").
		Select("chore_histories.*, chores.name as chore_name, circles.id as circle_id, time_sessions.duration, time_sessions.start_time, time_sessions.updated_at as timer_updated_at").
		Joins("LEFT JOIN chores ON chore_histories.chore_id = chores.id").
		Joins("LEFT JOIN circles ON chores.circle_id = circles.id").
		Joins("LEFT JOIN time_sessions ON chore_histories.id = time_sessions.chore_history_id").
		Joins("LEFT JOIN chore_assignees ON chores.id = chore_assignees.chore_id AND chore_assignees.user_id = ?", userID).
		Where("circles.id = ? AND chore_histories.updated_at > ?", circleID, since).
		Where("(chores.is_private = false) OR (chores.is_private = true AND (chores.created_by = ? OR chore_assignees.user_id = ?))", userID, userID).
		Order("chore_histories.performed_at desc, chore_histories.updated_at desc")

	if !includeCircle {
		query = query.Where("chore_histories.completed_by = ?", userID)
	}

	// Apply pagination
	query = query.Limit(maxRecords).Offset(offset)

	if err := query.Find(&chores).Error; err != nil {
		return nil, err
	}
	return chores, nil
}

func (r *ChoreRepository) UpdateChoreStatus(c context.Context, choreID int, status chModel.Status) error {
	return r.db.WithContext(c).Model(&chModel.Chore{}).Where("id = ?", choreID).Update("status", status).Error
}

func (r *ChoreRepository) GetActiveTimeSession(c context.Context, choreID int) (*chModel.TimeSession, error) {
	var session chModel.TimeSession
	if err := r.db.WithContext(c).Where("chore_id = ? AND status < 2", choreID).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // No active session found
		}
		return nil, err
	}
	return &session, nil
}

func (r *ChoreRepository) CreateTimeSession(c context.Context, chore *chModel.Chore, userID int) (*chModel.TimeSession, error) {
	log := logging.FromContext(c)
	var timeSession *chModel.TimeSession
	err := r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		ch := &chModel.ChoreHistory{
			ChoreID:     chore.ID,
			CompletedBy: userID,
			AssignedTo:  chore.AssignedTo,
			DueDate:     chore.NextDueDate,
			Status:      chModel.ChoreHistoryStatusStarted,
		}

		if err := tx.Create(ch).Error; err != nil {
			log.Errorf("Failed to create chore history: %v", err)
			return err
		}

		ts := &chModel.TimeSession{
			ChoreID:        chore.ID,
			ChoreHistoryID: ch.ID,
		}
		ts.Start(userID)
		if err := tx.Create(ts).Error; err != nil {
			log.Errorf("Failed to create time session: %v", err)
			return err
		}
		timeSession = ts
		return nil
	})
	return timeSession, err
}

func (r *ChoreRepository) UpdateTimeSession(c context.Context, session *chModel.TimeSession) error {
	return r.db.WithContext(c).Save(session).Error
}

func (r *ChoreRepository) CompleteTimeSession(c context.Context, session *chModel.TimeSession, chore *chModel.Chore, userID int) error {
	log := logging.FromContext(c)
	return r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		session.Finish(userID)
		if err := tx.Save(session).Error; err != nil {
			log.Errorf("Failed to complete time session: %v", err)
			return err
		}
		return nil
	})

}

func (r *ChoreRepository) GetTimeSessionsByChoreID(c context.Context, choreID int, choreHistoryId *int) (*chModel.TimeSession, error) {
	var session *chModel.TimeSession

	if choreHistoryId != nil {
		// Get sessions for specific chore history ID
		if err := r.db.WithContext(c).Where("chore_id = ? AND chore_history_id = ?", choreID, *choreHistoryId).Find(&session).Error; err != nil {
			return nil, err
		}
	} else {
		// Get sessions for the most recent chore history (based on due_date)
		query := r.db.WithContext(c).
			Table("time_sessions").
			Select("time_sessions.*").
			Joins("LEFT JOIN chore_histories ON time_sessions.chore_history_id = chore_histories.id").
			Where("time_sessions.chore_id = ?", choreID).
			Order("chore_histories.due_date DESC")

		if err := query.Find(&session).Error; err != nil {
			return nil, err
		}
	}

	return session, nil
}

func (r *ChoreRepository) GetTimeSessionByID(c context.Context, sessionID int) (*chModel.TimeSession, error) {
	var session chModel.TimeSession
	if err := r.db.WithContext(c).First(&session, sessionID).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *ChoreRepository) UpdateTimeSessionData(c context.Context, session *chModel.TimeSession) error {
	// Recalculate duration based on pause log
	session.Duration = 0
	for _, entry := range session.PauseLog {
		if entry.Duration > 0 {
			session.Duration += entry.Duration
		}
	}

	session.UpdateAt = time.Now().UTC()
	return r.db.WithContext(c).Save(session).Error
}

func (r *ChoreRepository) DeleteTimeSession(c context.Context, sessionID int, choreID int) error {
	return r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		// delete existing choreHistory linked to the time session:
		if err := tx.Where(
			"chore_id = ? and status = ?", choreID, chModel.ChoreHistoryStatusStarted,
		).Delete(&chModel.ChoreHistory{}).Error; err != nil {
			logging.FromContext(c).Errorf("Failed to delete chore history for session %d and chore %d: %v", sessionID, choreID, err)
			return err
		}
		// delete the time session itself:
		if err := tx.Where("id = ?", sessionID).Delete(&chModel.TimeSession{}).Error; err != nil {
			logging.FromContext(c).Errorf("Failed to delete time session %d for chore %d: %v", sessionID, choreID, err)
			return err
		}
		return nil

	})
	// delete where session ID matches and chore ID matches

}
