-- Leaderboard POC schema. Apply against the `leaderboard` database in the
-- running MySQL container — see README.md for the exact docker exec command.

CREATE TABLE IF NOT EXISTS users (
  id INT PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(100) NOT NULL,
  region VARCHAR(50) NOT NULL
);

CREATE TABLE IF NOT EXISTS friends (
  user_id INT NOT NULL,
  friend_id INT NOT NULL,
  PRIMARY KEY (user_id, friend_id)
);

CREATE TABLE IF NOT EXISTS score_events (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id INT NOT NULL,
  region VARCHAR(50) NOT NULL,
  period_type ENUM('daily','weekly','monthly') NOT NULL,
  period_key VARCHAR(20) NOT NULL,       -- e.g. '2026-09-07', '2026-W36', '2026-09'
  score_delta INT NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_period (user_id, period_type, period_key)
);

CREATE TABLE IF NOT EXISTS leaderboard_snapshots (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  scope ENUM('global','regional','friends') NOT NULL,
  region VARCHAR(50),                    -- null for global scope
  period_type ENUM('daily','weekly','monthly') NOT NULL,
  period_key VARCHAR(20) NOT NULL,
  rank_position INT NOT NULL,
  user_id INT NOT NULL,
  score INT NOT NULL,
  snapshot_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_lookup (scope, region, period_type, period_key)
);
