package ginboot

import (
	"database/sql"
	"fmt"
	"time"
)

type SQLConfig struct {
	Driver   string
	Host     string
	Port     int
	Username string
	Password string
	Database string
	Options  map[string]string
}

func NewSQLConfig() *SQLConfig {
	return &SQLConfig{
		Host:    "localhost",
		Port:    5432,
		Options: make(map[string]string),
	}
}

func (c *SQLConfig) WithDriver(driver string) *SQLConfig {
	c.Driver = driver
	return c
}

func (c *SQLConfig) WithCredentials(username, password string) *SQLConfig {
	c.Username = username
	c.Password = password
	return c
}

func (c *SQLConfig) WithHost(host string, port int) *SQLConfig {
	c.Host = host
	c.Port = port
	return c
}

func (c *SQLConfig) WithDatabase(database string) *SQLConfig {
	c.Database = database
	return c
}

func (c *SQLConfig) WithOption(key, value string) *SQLConfig {
	c.Options[key] = value
	return c
}

func (c *SQLConfig) BuildDSN() string {
	switch c.Driver {
	case "postgres":
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			c.Host, c.Port, c.Username, c.Password, c.Database)
	case "mysql":
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
			c.Username, c.Password, c.Host, c.Port, c.Database)
	default:
		return ""
	}
}

func (c *SQLConfig) Connect() (*sql.DB, error) {
	db, err := sql.Open(c.Driver, c.BuildDSN())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}

	return db, nil
}
