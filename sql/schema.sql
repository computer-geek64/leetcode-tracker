DROP VIEW IF EXISTS scoreboard;
DROP VIEW IF EXISTS valid_solution;
DROP TABLE IF EXISTS solution;
DROP TABLE IF EXISTS problem;
DROP TYPE IF EXISTS difficulty;


CREATE TYPE difficulty AS ENUM ('easy', 'medium', 'hard');

CREATE TABLE problem (
    id integer PRIMARY KEY,
    name varchar NOT NULL,
    slug varchar UNIQUE NOT NULL,
    difficulty difficulty NOT NULL
);

CREATE TABLE solution (
    id bigint PRIMARY KEY,
    problem_id bigint NOT NULL REFERENCES problem (id),
    username varchar NOT NULL,
    timestamp timestamp without time zone NOT NULL,
    language varchar NOT NULL
);

CREATE VIEW valid_solution AS (
    WITH RECURSIVE lag_cte AS (
        SELECT
            *,
            lag(id) OVER (PARTITION BY username, problem_id ORDER BY timestamp) AS last_id,
            lag(timestamp, 1, '-Infinity') OVER (PARTITION BY username, problem_id ORDER BY timestamp) AS last_timestamp
        FROM solution
    ),
    recursive_cte AS (
        SELECT
            *,
            timestamp AS last_valid_timestamp,
            true AS is_valid
        FROM lag_cte
        WHERE last_timestamp + INTERVAL '1 year' <= timestamp
        UNION ALL
        SELECT
            l.*,
            CASE
                WHEN r.last_valid_timestamp + INTERVAL '1 year' <= l.timestamp THEN l.timestamp
                ELSE r.last_valid_timestamp
            END AS last_valid_timestamp,
            r.last_valid_timestamp + INTERVAL '1 year' <= l.timestamp AS is_valid
        FROM lag_cte AS l
        INNER JOIN recursive_cte AS r
        ON l.last_id = r.id
        WHERE l.last_timestamp + INTERVAL '1 year' > l.timestamp
    )
    SELECT id, problem_id, username, timestamp, language
    FROM recursive_cte
    WHERE is_valid
);

CREATE VIEW scoreboard AS (
    WITH daily_cte AS (
        SELECT
            username,
            timestamp::date,
            count(*) AS problems,
            sum(
                CASE
                    WHEN p.difficulty = 'easy' THEN 1
                    WHEN p.difficulty = 'medium' THEN 2
                    WHEN p.difficulty = 'hard' THEN 5
                    ELSE NULL
                END
            ) AS daily_score
        FROM problem AS p
        INNER JOIN valid_solution AS s
        ON p.id = s.problem_id
        WHERE timestamp >= '2025-12-01'
        GROUP BY username, timestamp::date
    ),
    total_cte AS (
        SELECT
            username,
            count(*) FILTER (WHERE daily_score >= 2) AS days,
            sum(problems) AS problems,
            sum(daily_score) AS score
        FROM daily_cte
        GROUP BY username
    )
    SELECT *, rank() OVER (ORDER BY days, score, problems DESC) AS place
    FROM total_cte
    ORDER BY place
);
