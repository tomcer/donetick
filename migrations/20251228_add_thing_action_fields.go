package migrations

import (
	"context"

	"donetick.com/core/logging"
	"gorm.io/gorm"
)

type AddThingActionFields20251228 struct{}

func (m AddThingActionFields20251228) ID() string {
	return "20251228_add_thing_action_fields"
}

func (m AddThingActionFields20251228) Description() string {
	return "Add action_type and action_value fields to thing_chores table for automatic actions on chore completion"
}

func (m AddThingActionFields20251228) Down(ctx context.Context, db *gorm.DB) error {
	log := logging.FromContext(ctx)

	// Remove action fields if rolling back
	if db.Migrator().HasColumn(&ThingChore{}, "action_type") {
		if err := db.Migrator().DropColumn(&ThingChore{}, "action_type"); err != nil {
			log.Errorf("Failed to drop action_type column: %v", err)
			return err
		}
	}

	if db.Migrator().HasColumn(&ThingChore{}, "action_value") {
		if err := db.Migrator().DropColumn(&ThingChore{}, "action_value"); err != nil {
			log.Errorf("Failed to drop action_value column: %v", err)
			return err
		}
	}

	log.Info("Successfully rolled back thing_chores action fields")
	return nil
}

func (m AddThingActionFields20251228) Up(ctx context.Context, db *gorm.DB) error {
	log := logging.FromContext(ctx)

	// Add action_type column if it doesn't exist
	if !db.Migrator().HasColumn(&ThingChore{}, "action_type") {
		if err := db.Migrator().AddColumn(&ThingChore{}, "action_type"); err != nil {
			log.Errorf("Failed to add action_type column: %v", err)
			return err
		}
		log.Info("Added action_type column to thing_chores")
	}

	// Add action_value column if it doesn't exist
	if !db.Migrator().HasColumn(&ThingChore{}, "action_value") {
		if err := db.Migrator().AddColumn(&ThingChore{}, "action_value"); err != nil {
			log.Errorf("Failed to add action_value column: %v", err)
			return err
		}
		log.Info("Added action_value column to thing_chores")
	}

	log.Info("Successfully migrated thing_chores table with action fields")
	return nil
}

// ThingChore model reference for migration
type ThingChore struct {
	ThingID     int    `gorm:"column:thing_id;primaryKey"`
	ChoreID     int    `gorm:"column:chore_id;primaryKey"`
	ActionType  string `gorm:"column:action_type"`
	ActionValue string `gorm:"column:action_value"`
}

func (ThingChore) TableName() string {
	return "thing_chores"
}

// Register this migration
func init() {
	Register(AddThingActionFields20251228{})
}
