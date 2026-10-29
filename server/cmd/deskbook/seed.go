package main

import (
	"fmt"
	"time"

	"deskbook/internal/auth"
	"deskbook/internal/store"
)

// seedDocument builds the first data file: default settings, the floor plan
// desks from the design reference, and one administrator account.
func seedDocument(adminPassword string) (*store.Document, error) {
	hash, err := auth.HashPassword(adminPassword)
	if err != nil {
		return nil, fmt.Errorf("admin password: %w", err)
	}
	now := time.Now().UTC()
	return &store.Document{
		SchemaVersion: store.SchemaVersion,
		Revision:      1,
		UpdatedAt:     now,
		Settings: store.Settings{
			OfficeName: "Main Office", FloorName: "Floor 1", Timezone: "Europe/Amsterdam",
			WindowDays: 60, OpenHour: 7, CloseHour: 19, BackupRetention: 200,
		},
		Desks: seedDesks(),
		Employees: []store.Employee{{
			ID: "e-admin", Username: "admin", Name: "Administrator", Initials: "AD",
			Role: "admin", PasswordHash: hash, Active: true, CreatedAt: now,
		}},
		Bookings: []store.Booking{},
	}, nil
}

// Desk glyph coordinates are in the floor plan SVG's viewBox (0 0 1260 880).
// Rotation: 0 chair above desk, 90 chair right, 180 chair below, 270 chair left.
func seedDesks() []store.Desk {
	var desks []store.Desk
	add := func(zone string, x, y float64, rot int) {
		n := 1
		for _, d := range desks {
			if d.Zone == zone {
				n++
			}
		}
		desks = append(desks, store.Desk{
			ID: fmt.Sprintf("D-%02d", len(desks)+1), Label: fmt.Sprintf("%s%d", zone, n),
			Zone: zone, X: x, Y: y, Rotation: rot, Active: true,
		})
	}
	// 2x2 clusters facing each other across the desk divider (lower wing).
	horiz := func(zone string, x, y float64) {
		add(zone, x, y, 0)
		add(zone, x+58, y, 0)
		add(zone, x, y+24, 180)
		add(zone, x+58, y+24, 180)
	}
	// 2x2 clusters rotated 90 degrees (upper wing).
	vert := func(zone string, x, y float64) {
		add(zone, x, y, 270)
		add(zone, x+24, y, 90)
		add(zone, x, y+58, 270)
		add(zone, x+24, y+58, 90)
	}
	horiz("A", 430, 420)
	horiz("B", 54, 583)
	horiz("C", 428, 583)
	horiz("D", 54, 748)
	horiz("E", 430, 748)
	vert("F", 575, 222)
	vert("G", 762, 232)
	vert("H", 960, 244)
	add("I", 980, 92, 270)
	add("I", 1004, 92, 90)
	return desks
}
