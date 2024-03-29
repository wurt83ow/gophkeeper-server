package bdkeeper

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file" // registers a migrate driver.
	_ "github.com/jackc/pgx/v5/stdlib"                   // registers a pgx driver.
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Log represents a logging interface.
type Log interface {
	Info(string, ...zapcore.Field)
}

// BDKeeper represents a database keeper.
type BDKeeper struct {
	conn *sql.DB
	log  Log
}

// NewBDKeeper creates a new BDKeeper instance.
func NewBDKeeper(dsn func() string, log Log, db *sql.DB) (*BDKeeper, error) {
	addr := dsn()
	if addr == "" && db == nil {
		log.Info("database dsn is empty")
		return nil, errors.New("database dsn is empty")
	}

	// If a database is passed, use it, otherwise connect to a new database.
	var conn *sql.DB
	if db != nil {
		conn = db
	} else {
		var err error
		conn, err = sql.Open("pgx", dsn())
		if err != nil {
			log.Info("Unable to connect to database: ", zap.Error(err))
			return nil, err
		}
		driver, err := postgres.WithInstance(conn, new(postgres.Config))
		if err != nil {
			log.Info("error getting driver: ", zap.Error(err))
			return nil, err
		}

		dir, err := os.Getwd()
		if err != nil {
			log.Info("error getting getwd: ", zap.Error(err))
		}

		// Fix error test path
		mp := dir + "/migrations"

		var path string
		if _, err := os.Stat(mp); err != nil {
			path = "../../"
		}

		m, err := migrate.NewWithDatabaseInstance(
			fmt.Sprintf("file://%smigrations", path),
			"postgres",
			driver)
		if err != nil {
			log.Info("Error creating migration instance : ", zap.Error(err))
			return nil, err
		}

		err = m.Up()
		if err != nil {
			log.Info("Error while performing migration: ", zap.Error(err))
		}
	}

	log.Info("Connected!")

	return &BDKeeper{
		conn: conn,
		log:  log,
	}, nil
}

// Ping checks the connectivity to the PostgreSQL database and returns true if successful, otherwise false.
func (bdk *BDKeeper) Ping() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := bdk.conn.PingContext(ctx); err != nil {
		return false
	}

	return true
}

// Close closes the connection to the PostgreSQL database and returns true if successful, otherwise false.
func (bdk *BDKeeper) Close() bool {
	bdk.log.Info("Stop database")
	err := bdk.conn.Close()
	if err != nil {
		bdk.log.Info("Error closing database connection: ", zap.Error(err))
		return false
	}
	bdk.log.Info("All SQL queries are completed")
	return true
}

// UserExists checks if a user exists in the database.
func (bdk *BDKeeper) UserExists(ctx context.Context, username string) (bool, error) {
	// Query to check if the user exists in the database.
	query := `SELECT COUNT(*) FROM Users WHERE username = $1;`

	// Execute the query.
	row := bdk.conn.QueryRowContext(ctx, query, username)

	// Get the result.
	var count int
	err := row.Scan(&count)
	if err != nil {
		return false, err
	}

	// If the count is greater than 0, the user exists.
	return count > 0, nil
}

// AddUser adds a new user to the database.
func (bdk *BDKeeper) AddUser(ctx context.Context, username string, hashedPassword string) error {
	// Query to add a new user to the database.
	query := `INSERT INTO Users (username, password) VALUES ($1, $2);`

	// Execute the query.
	_, err := bdk.conn.ExecContext(ctx, query, username, hashedPassword)
	return err
}

// GetPassword retrieves the hashed password of a user from the database.
func (bdk *BDKeeper) GetPassword(ctx context.Context, username string) (string, error) {
	// Query to retrieve the hashed password of a user from the database.
	query := `SELECT password FROM Users WHERE username = $1;`

	// Execute the query.
	row := bdk.conn.QueryRowContext(ctx, query, username)

	// Get the result.
	var password string
	err := row.Scan(&password)
	if err != nil {
		return "", err
	}

	// Return the hashed password.
	return password, nil
}

// GetUserID retrieves the user ID of a user from the database.
func (bdk *BDKeeper) GetUserID(ctx context.Context, username string) (int, error) {
	// Query to retrieve the user ID of a user from the database.
	query := `SELECT id FROM Users WHERE username = $1;`

	// Execute the query.
	row := bdk.conn.QueryRowContext(ctx, query, username)

	// Get the result.
	var id int
	err := row.Scan(&id)
	if err != nil {
		return 0, err
	}

	// Return the user ID.
	return id, nil
}

// AddData adds data to a table in the database.
func (bdk *BDKeeper) AddData(ctx context.Context, table string, user_id int, entry_id string, data map[string]string) error {
	keys := make([]string, 0, len(data)+2)        // +2 for user_id and entry_id
	values := make([]interface{}, 0, len(data)+2) // +2 for user_id and entry_id

	// Add user_id and entry_id to the beginning of the lists of keys and values
	keys = append(keys, "user_id", "id")
	values = append(values, user_id, entry_id)

	for key, value := range data {
		keys = append(keys, key)
		values = append(values, value)
	}

	// Create placeholders for values
	placeholders := make([]string, len(values))
	for i := range values {
		placeholders[i] = "$" + strconv.Itoa(i+1)
	}

	stmt, err := bdk.conn.Prepare(fmt.Sprintf("INSERT INTO %s(%s) VALUES(%s)", table, strings.Join(keys, ","), strings.Join(placeholders, ",")))
	if err != nil {
		return err
	}
	_, err = stmt.ExecContext(ctx, values...)

	return err
}

// UpdateData updates data in a table in the database.
func (bdk *BDKeeper) UpdateData(ctx context.Context, table string, user_id int, entry_id string, data map[string]string) error {
	setClauses := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data)+2) // +2 для user_id и id

	i := 1
	for key, value := range data {
		setClauses = append(setClauses, key+" = $"+strconv.Itoa(i))
		values = append(values, value)
		i++
	}

	// Add user_id and id to the end of the list of values
	values = append(values, user_id, entry_id)

	stmt, err := bdk.conn.Prepare(fmt.Sprintf("UPDATE %s SET %s WHERE user_id = $%d AND id = $%d", table, strings.Join(setClauses, ","), i, i+1))
	if err != nil {
		return err
	}
	_, err = stmt.ExecContext(ctx, values...)
	return err
}

// DeleteData marks data as deleted in a table in the database and updates the 'updated_at' field.
func (bdk *BDKeeper) DeleteData(ctx context.Context, table string, user_id int, entry_id string) error {
	// Check user_id and table
	if user_id == 0 || table == "" {
		return errors.New("user_id and table must be specified")
	}

	// Check entry_id
	if entry_id == "" {
		return errors.New("entry_id must be specified")
	}

	// Prepare the query to update the record's deleted flag and 'updated_at' field
	updateQuery := fmt.Sprintf("UPDATE %s SET deleted = TRUE, updated_at = $1 WHERE user_id = $2 AND id = $3", table)
	args := []interface{}{time.Now().UTC(), user_id, entry_id}

	// Execute the query to update the record's deleted flag and 'updated_at' field
	_, err := bdk.conn.ExecContext(ctx, updateQuery, args...)
	return err
}

// GetAllData retrieves all data from a table in the database.
func (bdk *BDKeeper) GetAllData(ctx context.Context, table string, userID int, lastSync time.Time, inclDel bool) ([]map[string]string, error) {
	// Get all columns of the table
	rows, err := bdk.conn.QueryContext(ctx, fmt.Sprintf(`SELECT column_name FROM information_schema.columns WHERE table_name = '%s'`, strings.ToLower(table)))
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, fmt.Errorf("failed to scan column: %w", err)
		}
		cols = append(cols, col)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows encountered an error: %w", err)
	}

	// Build the condition for the query
	var condition string
	if !inclDel {
		condition += " AND deleted = false"
	}
	if !lastSync.IsZero() {
		condition += fmt.Sprintf(" AND updated_at > '%s'", lastSync.Format(time.RFC3339))
	}

	// Execute the query to fetch all data from the table for the given user ID considering the condition
	query := fmt.Sprintf("SELECT %s FROM %s WHERE user_id = $1%s", strings.Join(cols, ","), table, condition)
	rows, err = bdk.conn.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	values := make([]interface{}, len(cols))
	for i := range values {
		values[i] = new(sql.NullString)
	}

	var data []map[string]string
	for rows.Next() {
		if err := rows.Scan(values...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		row := make(map[string]string)
		for i, column := range cols {
			if ns, ok := values[i].(*sql.NullString); ok {
				row[column] = ns.String
			}
		}
		data = append(data, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows encountered an error: %w", err)
	}

	return data, nil
}
