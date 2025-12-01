package database

import "time"

type Difficulty string

const (
	DIFFICULTY_EASY Difficulty = "easy"
	DIFFICULTY_MEDIUM = "medium"
	DIFFICULTY_HARD = "hard"
)

type Problem struct {
	Id int
	Name string
	Slug string
	Difficulty Difficulty
}

type Solution struct {
	Id int64
	ProblemId int
	Username string
	Timestamp time.Time
	Language string
}

type ScoreboardEntry struct {
	Username string
	Days int
	Problems int
	Score int
	Streak int
	Place int
}
