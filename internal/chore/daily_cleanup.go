package chore

import (
	"context"
	"sync"
	"time"

	chModel "donetick.com/core/internal/chore/model"
	chRepo "donetick.com/core/internal/chore/repo"
	"donetick.com/core/logging"
	"gorm.io/gorm"
)

type DailyCleanupService struct {
	choreRepo       *chRepo.ChoreRepository
	lastRunPerCircle map[int]string // circleID -> date string "2006-01-02"
	mu              sync.RWMutex
}

func NewDailyCleanupService(
	choreRepo *chRepo.ChoreRepository,
) *DailyCleanupService {
	return &DailyCleanupService{
		choreRepo:       choreRepo,
		lastRunPerCircle: make(map[int]string),
	}
}

// CheckAndRunCleanup runs daily cleanup for a specific circle if not already run today
func (s *DailyCleanupService) CheckAndRunCleanup(ctx context.Context, circleID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log := logging.FromContext(ctx)
	todayDate := time.Now().UTC().Format("2006-01-02")

	// Check if cleanup already ran today for this circle
	if lastRun, exists := s.lastRunPerCircle[circleID]; exists && lastRun == todayDate {
		return nil // Already ran today
	}

	log.Infow("Running daily cleanup for circle", "circleID", circleID)

	// Find all active recurring tasks with nextDueDate < today's midnight
	now := time.Now().UTC()

	var overdueChores []*chModel.Chore
	err := s.choreRepo.GetDB().WithContext(ctx).
		Preload("Assignees").
		Where("circle_id = ?", circleID).
		Where("is_active = ?", true).
		Where("frequency_type NOT IN (?)", []string{"once", "no_repeat", "trigger"}).
		Where("next_due_date < ?", now).
		Find(&overdueChores).Error

	if err != nil {
		log.Errorw("Failed to get overdue chores", "error", err, "circleID", circleID)
		return err
	}

	rescheduledCount := 0
	for _, chore := range overdueChores {
		// Calculate next due date
		nextDueDate, err := scheduleNextDueDate(ctx, chore, chore.NextDueDate.UTC())
		if err != nil || nextDueDate == nil {
			log.Warnw("Failed to calculate next due date for chore", "choreID", chore.ID, "error", err)
			continue
		}

		// Find next assignee
		history, err := s.choreRepo.GetChoreHistory(ctx, chore.ID)
		if err != nil {
			log.Warnw("Failed to get chore history", "choreID", chore.ID, "error", err)
			continue
		}

		var nextAssignedTo int
		if chore.AssignedTo != nil {
			nextAssignedTo, err = checkNextAssignee(chore, history, *chore.AssignedTo)
		} else {
			nextAssignedTo, err = checkNextAssignee(chore, history, 0)
		}
		if err != nil {
			log.Warnw("Failed to determine next assignee", "choreID", chore.ID, "error", err)
			continue
		}

		// Reschedule without points
		err = s.choreRepo.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Update chore
			choreUpdates := map[string]interface{}{
				"next_due_date": nextDueDate,
				"assigned_to":   nextAssignedTo,
				"status":        chModel.ChoreStatusNoStatus,
			}

			if err := tx.Model(&chModel.Chore{}).Where("id = ?", chore.ID).Updates(choreUpdates).Error; err != nil {
				return err
			}

			// Create history record
			timeNow := time.Now().UTC()
			ch := &chModel.ChoreHistory{
				ChoreID:     chore.ID,
				PerformedAt: &timeNow,
				CompletedBy: 0, // System user
				AssignedTo:  chore.AssignedTo,
				DueDate:     chore.NextDueDate,
				Status:      chModel.ChoreHistoryStatusAutoSkipped,
				Points:      nil, // NO POINTS
			}

			return tx.Save(ch).Error
		})

		if err == nil {
			rescheduledCount++
			log.Infow("Auto-rescheduled overdue chore", "choreID", chore.ID, "newDueDate", nextDueDate)
		} else {
			log.Errorw("Failed to reschedule chore", "choreID", chore.ID, "error", err)
		}
	}

	// Mark cleanup as done for today
	s.lastRunPerCircle[circleID] = todayDate
	log.Infow("Daily cleanup completed", "circleID", circleID, "rescheduledChores", rescheduledCount)

	return nil
}
