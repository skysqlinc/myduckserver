# MyDuckServer Tools

A command-line tool for managing MyDuckServer operations.

## Commands

### copy-users

Copies MySQL users from a source MySQL server to a target MyDuck server.

#### Usage

```bash
myducktools copy-users <source-dsn> <target-dsn> [options]
```

#### Options

`--merge-users-by-name` (default: true): When enabled, merges users by name and creates them with wildcard host (`%`) instead of specific hosts

#### Examples

```bash
# Copy users from local MySQL to local MyDuck
myducktools copy-users 'root:password1@tcp(localhost:3306)/' 'root:password2@tcp(localhost:3307)/'

# Copy users without merging by name (preserves original host specifications)
myducktools copy-users 'root:password1@tcp(localhost:3306)/' 'root:password2@tcp(localhost:3307)/' --merge-users-by-name=false
```

#### Features

- Connects to source MySQL server and extracts all non-system users (excludes 'mariadb.sys', 'mysql', 'root')
- Retrieves CREATE USER statements and GRANT statements for each user
- Modifies the statements to make them MySQL compatible
  - The following privileges are filtered out as they are not supported by MyDuckServer:
    - `BINLOG MONITOR`
    - `DELETE HISTORY`
    - `SET USER`
    - `READ_ONLY ADMIN`
    - `FEDERATED ADMIN`
    - `BINLOG ADMIN`
    - `SLAVE MONITOR`
    - `CONNECTION ADMIN`
    - `REPLICATION MASTER ADMIN`
- Executes CREATE USER and GRANT statements on target server within transactions
- By default merges the users by name and creates them with wildcard host (`%`), because MyDuckServer currently has authentication issues when a specific IP is used as the host.

#### Supported DSN Format

The DSN (Data Source Name) format follows the Golang MySQL driver standard:

```text
username:password@tcp(host:port)/
```

**Important:** The trailing slash `/` is mandatory.
