// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package database

import (
	"fmt"
	"time"

	"github.com/jinzhu/gorm"
)

type AuditList struct {
	Entries []*AuditEntry

	PreloadedRealms map[uint]*Realm
	PreloadedUsers  map[uint]*User
}

// LoadAuditList loads the audit list from the given list of audit entries,
// handling preloading for polymorphic associations.
func (db *Database) LoadAuditList(entries []*AuditEntry) (*AuditList, error) {
	// Start building
	var l AuditList
	l.Entries = entries
	l.PreloadedRealms = make(map[uint]*Realm)
	l.PreloadedUsers = make(map[uint]*User)

	// Build a map of all IDs and resource types so we can preload all the
	// associations. This makes me miss Rails...
	lookups := make(map[string][]uint)
	for _, e := range entries {
		if _, ok := lookups[e.TargetType]; !ok {
			lookups[e.TargetType] = []uint{}
		}
		lookups[e.TargetType] = append(lookups[e.TargetType], e.TargetID)

		if _, ok := lookups[e.SourceType]; !ok {
			lookups[e.SourceType] = []uint{}
		}
		lookups[e.SourceType] = append(lookups[e.SourceType], e.SourceID)
	}

	for k, v := range lookups {
		switch k {
		case "realms":
			var realms []*Realm
			if err := db.db.
				Model(&Realm{}).
				Where("id IN (?)", v).
				Find(&realms).
				Error; err != nil {
				return nil, fmt.Errorf("failed to load realms: %w", err)
			}

			for _, v := range realms {
				l.PreloadedRealms[v.ID] = v
			}
		case "users":
			var users []*User
			if err := db.db.
				Model(&User{}).
				Where("id IN (?)", v).
				Find(&users).
				Error; err != nil {
				return nil, fmt.Errorf("failed to load users: %w", err)
			}

			for _, v := range users {
				l.PreloadedUsers[v.ID] = v
			}
		default:
			return nil, fmt.Errorf("unknown polymorphic association %v", k)
		}
	}

	return &l, nil
}

type AuditEntry struct {
	Errorable

	// ID is the entry's ID.
	ID uint `gorm:"primary_key;"`

	// UserID is the user that performed this action.
	UserID uint `gorm:"column:user_id; type:integer; not null;"`
	User   *User

	// Action is the auditable action.
	Action string `gorm:"column:action; type:varchar(50); not null;"`

	// Target is the entity that is being acted upon. It should always be present.
	// For example, if the audit was "Susan deleted Seth", Susan would be the user
	// and Seth would be the target.
	TargetType string `gorm:"column:target_type; type:varchar(75); not null;"`
	TargetID   uint   `gorm:"column:target_id; type:integer; not null;"`

	// Source is the entity upon which the target was acted, if any. It could be
	// nil depending on the source. For example, if the audit was "Susan deleted
	// Seth", there will be no source. But if the audit were "Susan removed Seth
	// from Narnia", then the source would be Narnia.
	SourceType string `gorm:"column:source_type; type:varchar(75);"`
	SourceID   uint   `gorm:"column:source_id; type:integer;"`

	// CreatedAt is when the entry was created.
	CreatedAt time.Time
}

// PurgeAuditEntries will delete audit entries which were created longer than
// maxAge ago.
func (db *Database) PurgeAuditEntries(maxAge time.Duration) (int64, error) {
	if maxAge > 0 {
		maxAge = -1 * maxAge
	}
	createdBefore := time.Now().UTC().Add(maxAge)

	result := db.db.
		Unscoped().
		Where("created_at < ?", createdBefore).
		Delete(&AuditEntry{})
	return result.RowsAffected, result.Error
}

func (db *Database) SaveAuditEntry(e *AuditEntry) error {
	return SaveAuditEntry(db.db, e)
}

func SaveAuditEntry(tx *gorm.DB, e *AuditEntry) error {
	return tx.Save(e).Error
}
