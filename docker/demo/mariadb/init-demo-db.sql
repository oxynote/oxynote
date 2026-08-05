CREATE DATABASE IF NOT EXISTS demodb;
CREATE USER IF NOT EXISTS 'demouser'@'%' IDENTIFIED BY 'demopass';
GRANT ALL PRIVILEGES ON demodb.* TO 'demouser'@'%';

CREATE USER IF NOT EXISTS 'demouser_ro'@'%' IDENTIFIED BY 'demopass';
GRANT SELECT ON demodb.* TO 'demouser_ro'@'%';

FLUSH PRIVILEGES;

USE demodb;

CREATE TABLE deployments (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    time DATETIME NOT NULL,
    service VARCHAR(255) NOT NULL,
    environment VARCHAR(255) NOT NULL,
    duration_seconds DOUBLE NOT NULL,
    success BOOLEAN NOT NULL,
    rollback BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE incidents (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    time DATETIME NOT NULL,
    severity VARCHAR(255) NOT NULL,
    service VARCHAR(255) NOT NULL,
    time_to_detect_minutes DOUBLE NOT NULL,
    time_to_resolve_minutes DOUBLE NOT NULL
);

CREATE TABLE build_metrics (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    time DATETIME NOT NULL,
    repository VARCHAR(255) NOT NULL,
    branch VARCHAR(255) NOT NULL,
    duration_seconds DOUBLE NOT NULL,
    test_count INT NOT NULL,
    tests_failed INT NOT NULL,
    coverage_pct DOUBLE NOT NULL
);

CREATE INDEX idx_deployments_time ON deployments (time);
CREATE INDEX idx_incidents_time ON incidents (time);
CREATE INDEX idx_build_metrics_time ON build_metrics (time);
