package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMakeCreateUserCompatible(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"IDENTIFIED BY PASSWORD is fixed": {
			input: "CREATE USER 'testuser'@'localhost' IDENTIFIED BY PASSWORD 'password'",
			want:  "CREATE USER IF NOT EXISTS 'testuser'@'localhost' IDENTIFIED WITH mysql_native_password AS 'password'",
		},
		"No IDENTIFIED BY PASSWORD": {
			input: "CREATE USER 'foo'@'bar'",
			want:  "CREATE USER IF NOT EXISTS 'foo'@'bar'",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			result := makeCreateUserCompatible(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestMakeGrantStatementsCompatible(t *testing.T) {
	tests := map[string]struct {
		input []string
		want  []string
	}{
		"Grants with IDENTIFIED BY PASSWORD are fixed": {
			input: []string{
				"GRANT SELECT, INSERT ON testdb.* TO 'testuser'@'localhost' IDENTIFIED BY PASSWORD 'hash'",
				"GRANT ALL PRIVILEGES ON *.* TO 'admin'@'%' WITH MAX_USER_CONNECTIONS 10",
				"GRANT BINLOG MONITOR ON *.* TO 'user'@'localhost'",
			},
			want: []string{
				"GRANT SELECT, INSERT ON testdb.* TO 'testuser'@'localhost'",
				"GRANT ALL PRIVILEGES ON *.* TO 'admin'@'%'",
			},
		},
		"Grants with REPLICATION SLAVE ADMIN are fixed": {
			input: []string{
				"GRANT SELECT, BINLOG MONITOR, REPLICATION SLAVE ADMIN ON *.* TO 'user'@'localhost'",
			},
			want: []string{
				"GRANT SELECT, REPLICATION_SLAVE_ADMIN ON *.* TO 'user'@'localhost'",
			},
		},
		"Unsupported grants are removed": {
			input: []string{
				"GRANT SELECT, BINLOG MONITOR, SUPER ON *.* TO 'user'@'localhost'",
			},
			want: []string{
				"GRANT SELECT, SUPER ON *.* TO 'user'@'localhost'",
			},
		},
		"Grant lines with all unsupported grants are removed": {
			input: []string{
				"GRANT SELECT ON *.* TO 'user'@'localhost'",
				"GRANT SET USER, BINLOG MONITOR ON *.* TO 'user'@'localhost'",
			},
			want: []string{
				"GRANT SELECT ON *.* TO 'user'@'localhost'",
			},
		},
		"Non-grant lines are removed": {
			input: []string{
				"SELECT * FROM users",
			},
			want: []string{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			result := makeGrantStatementsCompatible(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}
