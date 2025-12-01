package leetcode

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"
)

import (
	"github.com/computer-geek64/leetcode-tracker/config"
	"github.com/computer-geek64/leetcode-tracker/database"
)


const _AUTO_REFRESH_INTERVAL time.Duration = 6 * time.Hour
const _REFRESH_QUIET_PERIOD time.Duration = 5 * time.Minute


type Worker struct {
	config config.Config
	requests chan bool
	scheduler *time.Timer
	lastRefresh time.Time
	lastRefreshMutex sync.RWMutex
	csrfToken string
	httpClient *http.Client
	db *sql.DB
}

func NewWorker(conf config.Config, db *sql.DB) *Worker {
	return &Worker{
		config: conf,
		requests: make(chan bool),
		scheduler: time.NewTimer(_AUTO_REFRESH_INTERVAL),
		httpClient: &http.Client{},
		db: db,
	}
}

func (w *Worker) Start() {
	go w.run()
}

func (w *Worker) run() {
	if err := w.configureCsrfToken(); err != nil {
		panic(err)
	}
	slog.Info("Obtained CSRF token", "csrf", w.csrfToken)
	if err := w.refresh(); err != nil {
		panic(err)
	}

	for {
		select {
		case <- w.requests:
		case refreshTime := <- w.scheduler.C:
			var wait = time.Until(refreshTime.Add(_AUTO_REFRESH_INTERVAL))
			if w.scheduler.Reset(wait) {
				slog.Warn("Timer was reset while still active")
			}
		}
		if w.IsRateLimited() {
			continue
		}

		if err := w.refresh(); err != nil {
			continue
		}
	}
}

func (w *Worker) RequestRefresh() bool {
	select {
	case w.requests <- true:
		return true
	default:
		return false
	}
}

func (w *Worker) GetLastRefresh() time.Time {
	w.lastRefreshMutex.RLock()
	defer w.lastRefreshMutex.RUnlock()
	return w.lastRefresh
}

func (w *Worker) IsRateLimited() bool {
	return time.Now().Before(w.GetLastRefresh().Add(_REFRESH_QUIET_PERIOD))
}

func (w *Worker) refresh() error {
	var titleSlugs []string
	var titleSlugsSet = make(map[string]bool)
	var acSubmissionLists = make(map[string][]recentAcSubmission, len(w.config.Users))
	for username := range w.config.Users {
		var profile, userProfileErr = w.getUserProfile(username)
		if userProfileErr != nil {
			return userProfileErr
		}

		acSubmissionLists[username] = profile.RecentAcSubmissionList
		for _, acSubmission := range profile.RecentAcSubmissionList {
			if _, exists := titleSlugsSet[acSubmission.TitleSlug]; !exists {
				titleSlugsSet[acSubmission.TitleSlug] = true
				titleSlugs = append(titleSlugs, acSubmission.TitleSlug)
			}
		}
	}
	var questions, questionsErr = w.getQuestions(titleSlugs)
	if questionsErr != nil {
		return questionsErr
	}

	var problemIdBySlug = make(map[string]int, len(*questions))
	var problems = make([]database.Problem, 0, len(*questions))
	for titleSlug, question := range *questions {
		var id, idErr = strconv.Atoi(question.QuestionFrontendId)
		if idErr != nil {
			slog.Error("Failed to parse ID string as integer")
			return idErr
		}
		problemIdBySlug[titleSlug] = id

		var difficulty database.Difficulty
		switch question.Difficulty {
		case "Easy":
			difficulty = database.DIFFICULTY_EASY
		case "Medium":
			difficulty = database.DIFFICULTY_MEDIUM
		case "Hard":
			difficulty = database.DIFFICULTY_HARD
		default:
			slog.Error("Unknown problem difficulty", "difficulty", question.Difficulty)
			return fmt.Errorf("Unknown problem difficulty %s", question.Difficulty)
		}

		var problem = database.Problem{
			Id: id,
			Name: question.Title,
			Slug: titleSlug,
			Difficulty: difficulty,
		}
		problems = append(problems, problem)
	}

	var solutions []database.Solution
	for username, acSubmissionList := range acSubmissionLists {
		for _, acSubmission := range acSubmissionList {
			var ts, tsErr = strconv.ParseInt(acSubmission.Timestamp, 10, 64)
			if tsErr != nil {
				slog.Error("Failed to parse timestamp string as integer")
				return tsErr
			}

			var id, idErr = strconv.ParseInt(acSubmission.Id, 10, 64)
			if idErr != nil {
				slog.Error("Failed to parse ID as integer")
				return idErr
			}

			var solution = database.Solution{
				Id: id,
				ProblemId: problemIdBySlug[acSubmission.TitleSlug],
				Username: username,
				Timestamp: time.Unix(ts, 0).UTC(),
				Language: acSubmission.Lang,
			}
			solutions = append(solutions, solution)
		}
	}

	if err := database.InsertProblemsAndSolutions(w.db, problems, solutions); err != nil {
		return err
	}

	var now = time.Now()
	slog.Info("Refreshed!")
	w.lastRefreshMutex.Lock()
	w.lastRefresh = now
	w.lastRefreshMutex.Unlock()
	return nil
}
