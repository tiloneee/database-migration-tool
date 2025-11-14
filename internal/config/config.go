package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds all configuration for the migration tool
type Config struct {
	Remote   DatabaseConfig `mapstructure:"remote"`
	Local    DatabaseConfig `mapstructure:"local"`
	Docker   DockerConfig   `mapstructure:"docker"`
	Migration MigrationConfig `mapstructure:"migration"`
	Logging  LoggingConfig  `mapstructure:"logging"`
}

// DatabaseConfig represents database connection settings
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	SSLMode  string `mapstructure:"sslmode"`
}

// DockerConfig represents Docker settings
type DockerConfig struct {
	ContainerName string `mapstructure:"container_name"`
	ComposeFile   string `mapstructure:"compose_file"`
	AutoStart     bool   `mapstructure:"auto_start"`
}

// MigrationConfig represents migration behavior settings
type MigrationConfig struct {
	Anonymize      bool     `mapstructure:"anonymize"`
	TruncateTables bool     `mapstructure:"truncate_tables"`
	Tables         []string `mapstructure:"tables"`
	ExcludeTables  []string `mapstructure:"exclude_tables"`
	BatchSize      int      `mapstructure:"batch_size"`
}

// LoggingConfig represents logging settings
type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	OutputPath string `mapstructure:"output_path"`
	Format     string `mapstructure:"format"` // json or console
}

// ConnectionString generates a PostgreSQL connection string
func (db *DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		db.Host, db.Port, db.User, db.Password, db.Database, db.SSLMode,
	)
}

// DSN generates a PostgreSQL DSN for drivers that use it
func (db *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		db.User, db.Password, db.Host, db.Port, db.Database, db.SSLMode,
	)
}

// Load reads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read from config file if provided
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}
		}
	} else {
		// Look for config.yaml in current directory
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath(filepath.Join(os.Getenv("HOME"), ".database-migration-tool"))
		
		// Ignore error if config file doesn't exist
		_ = v.ReadInConfig()
	}

	// Environment variables override config file
	v.SetEnvPrefix("DBMIGRATE")
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Remote database defaults
	v.SetDefault("remote.host", "localhost")
	v.SetDefault("remote.port", 5432)
	v.SetDefault("remote.sslmode", "disable")

	// Local database defaults
	v.SetDefault("local.host", "localhost")
	v.SetDefault("local.port", 5433)
	v.SetDefault("local.database", "local_db")
	v.SetDefault("local.user", "postgres")
	v.SetDefault("local.password", "postgres")
	v.SetDefault("local.sslmode", "disable")

	// Docker defaults
	v.SetDefault("docker.container_name", "postgres-local")
	v.SetDefault("docker.compose_file", "docker-compose.yml")
	v.SetDefault("docker.auto_start", true)

	// Migration defaults
	v.SetDefault("migration.anonymize", false)
	v.SetDefault("migration.truncate_tables", true)
	v.SetDefault("migration.batch_size", 1000)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.output_path", "stdout")
	v.SetDefault("logging.format", "console")
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate remote database
	if c.Remote.Host == "" {
		return fmt.Errorf("remote.host is required")
	}
	if c.Remote.Database == "" {
		return fmt.Errorf("remote.database is required")
	}
	if c.Remote.User == "" {
		return fmt.Errorf("remote.user is required")
	}

	// Validate local database
	if c.Local.Host == "" {
		return fmt.Errorf("local.host is required")
	}
	if c.Local.Database == "" {
		return fmt.Errorf("local.database is required")
	}

	// Validate batch size
	if c.Migration.BatchSize <= 0 {
		return fmt.Errorf("migration.batch_size must be greater than 0")
	}

	return nil
}
