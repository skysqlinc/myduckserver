package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/alecthomas/kong"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var cli struct {
	CopyUsers struct {
		SourceDSN        string `arg:"" required:"true" help:"Source DSN"`
		TargetDSN        string `arg:"" required:"true" help:"Target DSN"`
		MergeUsersByName bool   `default:"true" help:"Merge users by name"`
	} `cmd:"copy-users" help:"Copy users from source to target"`
}

type User struct {
	User string `db:"User"`
	Host string `db:"Host"`
}

func main() {
	ctx := kong.Parse(&cli,
		kong.Name("myducktools"),
		kong.Description("MyDuckServer tools"),
		kong.UsageOnError())
	switch ctx.Command() {
	case "copy-users <source-dsn> <target-dsn>":
		if err := copyUsers(cli.CopyUsers.SourceDSN, cli.CopyUsers.TargetDSN, cli.CopyUsers.MergeUsersByName); err != nil {
			fmt.Printf("Error copying users: %v\n", err)
			os.Exit(1)
		}
	}
}

func copyUsers(sourceDSN, targetDSN string, mergeUsersByName bool) error {
	sourceDB, err := sqlx.Open("mysql", sourceDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to source database: %w", err)
	}
	defer sourceDB.Close()

	if err := sourceDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping source database: %w", err)
	}

	targetDB, err := sqlx.Open("mysql", targetDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to target database: %w", err)
	}
	defer targetDB.Close()

	if err := targetDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping target database: %w", err)
	}

	users, err := getUsers(sourceDB)
	if err != nil {
		return fmt.Errorf("failed to get users from source: %w", err)
	}

	fmt.Printf("Found %d users to copy\n", len(users))

	processedUsers := make(map[string]bool)
	for _, user := range users {
		if _, ok := processedUsers[user.User]; ok && mergeUsersByName {
			continue
		}

		userOnHost := fmt.Sprintf("`%s`@`%s`", user.User, user.Host)
		userStatements, err := prepareUserStatements(sourceDB, user)
		if err != nil {
			fmt.Printf("Warning: Failed to prepare user statements for %s: %v\n", userOnHost, err)
			continue
		}

		if mergeUsersByName {
			for i, stmt := range userStatements {
				userStatements[i] = strings.ReplaceAll(stmt, userOnHost, fmt.Sprintf("`%s`@`%%`", user.User))
			}
		}

		// Debug
		// fmt.Printf("Prepared user statements:\n%#v\n", userStatements)

		tx, err := targetDB.BeginTx(context.Background(), nil)
		if err != nil {
			fmt.Printf("Warning: Failed to begin transaction for user %s: %v\n", userOnHost, err)
			continue
		}

		for _, stmt := range userStatements {
			if _, err := tx.Exec(stmt); err != nil {
				// Debug
				// fmt.Printf("Warning: Failed to execute statement %s: %v\n", stmt, err)
				fmt.Printf("Warning: Failed to copy user %s: %v\n", userOnHost, err)
				tx.Rollback()
				break
			}
		}

		if err := tx.Commit(); err != nil {
			fmt.Printf("Warning: Failed to commit transaction for user %s: %v\n", userOnHost, err)
			continue
		}

		// Debug
		// fmt.Printf("Successfully copied user: %s\n", userOnHost)
		processedUsers[user.User] = true
	}

	if _, err := targetDB.Exec("FLUSH PRIVILEGES"); err != nil {
		return fmt.Errorf("failed to flush privileges: %w", err)
	}

	return nil
}

// Get all non-system users from a database.
func getUsers(db *sqlx.DB) ([]User, error) {
	query := `
		SELECT user, host FROM mysql.user WHERE user NOT IN ('mariadb.sys','mysql','root')`

	users := []User{}
	err := db.Select(&users, query)
	return users, err
}

// Prepare user creation and grant statements.
// Because there are inconsistencies in the way users are created in MySQL and MariaDB,
// we need to modify the statements to work with the MySQL syntax.
// Also, currently MyDuckServer has issues when connecting with user@host,
// so we need to create the user with the wildcard (%) as host.
func prepareUserStatements(sourceDB *sqlx.DB, user User) ([]string, error) {
	userOnHost := fmt.Sprintf("`%s`@`%s`", user.User, user.Host)
	createUserStmt, err := getCreateUserStatement(sourceDB, userOnHost)
	if err != nil {
		return nil, fmt.Errorf("failed to get CREATE USER statement for %s: %w", userOnHost, err)
	}

	grantStmts, err := getGrantStatements(sourceDB, userOnHost)
	if err != nil {
		return nil, fmt.Errorf("failed to get GRANT statements for %s: %w", userOnHost, err)
	}

	createUserStmt = makeCreateUserCompatible(createUserStmt)

	grantStmts = makeGrantStatementsCompatible(grantStmts)

	userStatements := []string{createUserStmt}
	userStatements = append(userStatements, grantStmts...)
	return userStatements, nil
}

func getCreateUserStatement(db *sqlx.DB, user string) (string, error) {
	var createUserStmt string
	err := db.Get(&createUserStmt, fmt.Sprintf("SHOW CREATE USER %s", user))
	return createUserStmt, err
}

func getGrantStatements(db *sqlx.DB, user string) ([]string, error) {
	var grants []string
	err := db.Select(&grants, fmt.Sprintf("SHOW GRANTS FOR %s", user))
	return grants, err
}

func makeCreateUserCompatible(createUserStmt string) string {
	createUserStmt = strings.Replace(createUserStmt, "CREATE USER", "CREATE USER IF NOT EXISTS", 1)
	createUserStmt = strings.Replace(createUserStmt, "IDENTIFIED BY PASSWORD", "IDENTIFIED WITH mysql_native_password AS", 1)
	return createUserStmt
}

var unsupportedGrants = map[string]bool{
	`BINLOG MONITOR`:           true,
	`DELETE HISTORY`:           true,
	`SET USER`:                 true,
	`READ_ONLY ADMIN`:          true,
	`FEDERATED ADMIN`:          true,
	`BINLOG ADMIN`:             true,
	`SLAVE MONITOR`:            true,
	`CONNECTION ADMIN`:         true,
	`REPLICATION MASTER ADMIN`: true,
}

func makeGrantStatementsCompatible(grantStmts []string) []string {
	compatibleGrants := []string{}

	for _, stmt := range grantStmts {
		// "IDENTIFIED BY PASSWORD" is part of CREATE USER only, not GRANT in MySQL.
		// While in MariaDB, it can be in both.
		stmt = regexp.MustCompile(` IDENTIFIED BY PASSWORD '.*'`).ReplaceAllString(stmt, "")

		// "WITH MAX_USER_CONNECTIONS" is part of CREATE USER only, not GRANT in MySQL.
		// While in MariaDB, it can be in both.
		stmt = regexp.MustCompile(` WITH MAX_USER_CONNECTIONS [0-9]+`).ReplaceAllString(stmt, "")

		grantsRegex := regexp.MustCompile(`^GRANT (.*) ON (.*)$`)
		matches := grantsRegex.FindStringSubmatch(stmt)
		if len(matches) == 0 {
			continue
		}
		grantsInStmt := strings.Split(matches[1], ", ")

		// Remove unsupported GRANTs
		supportedGrants := []string{}
		for _, grant := range grantsInStmt {
			if !unsupportedGrants[grant] {
				supportedGrants = append(supportedGrants, grant)
			}
		}
		if len(supportedGrants) == 0 {
			continue
		}
		stmt = "GRANT " + strings.Join(supportedGrants, ", ") + " ON " + matches[2]

		// For some reason for "REPLICATION SLAVE ADMIN" we need to use underscores,
		// while for other multi-word grants spaces are fine.
		stmt = strings.ReplaceAll(stmt, "REPLICATION SLAVE ADMIN", "REPLICATION_SLAVE_ADMIN")

		compatibleGrants = append(compatibleGrants, stmt)
	}

	return compatibleGrants
}
