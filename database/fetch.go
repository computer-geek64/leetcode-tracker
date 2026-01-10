package database

import (
	"database/sql"
	"log/slog"
	"time"
)


const scoreboardQuery = `WITH daily_cte AS (
	SELECT
		s.username,
		s.timestamp::date AS date,
		count(*) AS problems,
		sum(
			CASE
				WHEN p.difficulty = 'easy' THEN 1
				WHEN p.difficulty = 'medium' THEN 2
				WHEN p.difficulty = 'hard' THEN 4
				ELSE NULL
			END
		) AS daily_score
	FROM problem AS p
	INNER JOIN valid_solution AS s
	ON p.id = s.problem_id
	WHERE s.timestamp >= $1
	GROUP BY username, date
),
total_cte AS (
	SELECT
		username,
		count(*) FILTER (WHERE daily_score >= 2) AS days,
		sum(problems) AS problems,
		sum(daily_score) AS raw_score
	FROM daily_cte
	GROUP BY username
),
row_number_cte AS (
	SELECT
		username,
		date,
		row_number() OVER (PARTITION BY username ORDER BY date DESC) AS rn
	FROM daily_cte
	WHERE daily_score >= 2
),
streak_cte AS (
	SELECT
		username,
		max(rn) AS streak
	FROM row_number_cte
	WHERE date >= (current_timestamp AT TIME ZONE 'UTC' - rn * INTERVAL '1 day')::date
	GROUP BY username
)
SELECT
	username,
	days,
	problems,
	raw_score,
	days * 0.5 + raw_score * 0.5 AS weighted_score,
	coalesce(streak, 0) AS streak,
	rank() OVER (ORDER BY days * 0.5 + raw_score * 0.5 DESC, days DESC, raw_score DESC, problems DESC, streak DESC) AS place
FROM total_cte
LEFT JOIN streak_cte
USING (username)
ORDER BY place;`

func GetScoreboard(db *sql.DB, startDate time.Time) ([]ScoreboardEntry, error) {
	var rows, queryErr = db.Query(scoreboardQuery, startDate)
	if queryErr != nil {
		slog.Error("Failed to query scoreboard from database")
		return nil, queryErr
	}
	defer rows.Close()

	var scoreboard []ScoreboardEntry
	for rows.Next() {
		var scoreboardEntry ScoreboardEntry
		if err := rows.Scan(&scoreboardEntry.Username, &scoreboardEntry.Days, &scoreboardEntry.Problems, &scoreboardEntry.RawScore, &scoreboardEntry.WeightedScore, &scoreboardEntry.Streak, &scoreboardEntry.Place); err != nil {
			slog.Error("Failed to fetch values from query result")
			return nil, err
		}
		scoreboard = append(scoreboard, scoreboardEntry)
	}
	if err := rows.Err(); err != nil {
		slog.Error("Failed to fetch values from query result")
		return nil, err
	}

	return scoreboard, nil
}
