package main

import "time"

type joinRecord struct {
	UserID       string
	ServerID     string
	Region       string
	JoinToken    string
	CreationTime time.Time
	Checked      bool
}
