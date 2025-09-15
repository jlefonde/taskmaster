![taskmaster](https://socialify.git.ci/jlefonde/taskmaster/image?forks=1&issues=1&language=1&name=1&owner=1&pattern=Transparent&pulls=1&stargazers=1&theme=Auto)
# Taskmaster 🚀

A process supervision system written in Go, inspired by supervisord. Taskmaster monitors and manages long-running processes, automatically restarting them when necessary and providing a command-line interface for process control.

Unlike traditional supervisor systems that separate the daemon and client, Taskmaster provides a single integrated binary (taskmasterd) that includes both the supervisor and an embedded interactive controller. The system currently runs in the foreground (and don't offer daemonization of the supervisor for now), providing immediate access to the command-line interface for process management.

## ✨ Features

- **Process Management**: Start, stop, restart, and monitor multiple processes
- **Automatic Restart**: Configurable restart policies (never, always, on-failure)
- **Interactive Shell**: Built-in command-line interface with tab completion
- **Configuration Hot-Reload**: Update configuration without stopping the supervisor
- **Logging**: Comprehensive logging with configurable levels and output destinations
- **User Management**: Run processes as different users with proper privilege dropping
- **Signal Handling**: Graceful shutdown and configuration reloading via signals
- **Process Groups**: Support for multiple instances of the same program

## 📦 Installation

### Prerequisites

- Go 1.25.0 or later
- Root privileges (required for privilege de-escalation)

### Build from Source

```bash
git clone https://github.com/jlefonde/taskmaster.git
cd taskmaster
make
```

The binary will be available at `./bin/taskmasterd`.

## ⚙️ Configuration

Taskmaster uses YAML configuration files. The supervisor looks for configuration files in the following order:

1. Path specified with `-c` or `--configuration` flag
2. `../etc/taskmasterd.yaml` (relative to binary)
3. `../taskmasterd.yaml` (relative to binary)
4. `./taskmasterd.yaml` (current directory)
5. `./etc/taskmasterd.yaml` (current directory)
6. `/etc/taskmasterd.yaml`
7. `/etc/taskmaster/taskmasterd.yaml`

### Example Configuration

```yaml
taskmasterd:
    nocleanup: false
    childlogdir: /var/log/taskmasterd/
    logfile: /var/log/taskmasterd/taskmasterd.log
    loglevel: info

programs:
    nginx:
        cmd: /usr/sbin/nginx -g "daemon off;"
        numprocs: 1
        autostart: true
        autorestart: always
        exitcodes:
            - 0
        startretries: 3
        startsecs: 5
        stopsignal: TERM
        stopsecs: 10
        user: www-data
        env:
            PATH: /usr/local/bin:/usr/bin:/bin
        stdout_logfile: auto
        stderr_logfile: auto

    worker:
        cmd: /app/worker --config=/etc/worker.conf
        numprocs: 4
        autostart: true
        autorestart: on-failure
        exitcodes: [0]
        startretries: 3
        startsecs: 1
        stopsignal: USR1
        stopsecs: 2
        user: app
        workingdir: /app
        umask: 044
        env:
            LOG_LEVEL: "info"
```

### Configuration Options

#### Taskmasterd Section

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `nocleanup` | bool | false | Skip cleanup of old log files on startup |
| `childlogdir` | string | `/tmp` | Directory for child process log files |
| `logfile` | string | `./taskmasterd.log` | Path to supervisor log file |
| `loglevel` | string | `info` | Logging level (critical, error, warning, info, debug) |

#### Programs Section

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `cmd` | string | **required** | Command to execute |
| `numprocs` | int | 1 | Number of process instances to start |
| `umask` | int | - | Umask for the process |
| `workingdir` | string | - | Working directory for the process |
| `user` | string | `root` | User to run the process as |
| `autostart` | bool | true | Start process automatically when supervisor starts |
| `autorestart` | string | `on-failure` | Restart policy (never, always, on-failure) |
| `exitcodes` | []int | [0] | Expected exit codes |
| `startretries` | int | 3 | Number of restart attempts before giving up |
| `startsecs` | int | 1 | Time process must stay running to be considered started |
| `stopsignal` | string | `TERM` | Signal to send when stopping process |
| `stopsecs` | int | 10 | Time to wait before sending SIGKILL |
| `stdout_logfile` | string | `auto` | Path for stdout logs (`auto`, `none`, or file path) |
| `stderr_logfile` | string | `auto` | Path for stderr logs (`auto`, `none`, or file path) |
| `env` | map | - | Environment variables for the process |

## 🖥️ Usage

### Starting the Supervisor

```bash
# Start with default configuration
sudo ./bin/taskmasterd

# Start with custom configuration
sudo ./bin/taskmasterd -c /path/to/config.yaml

# Set log level and redirect log to syslog
sudo ./bin/taskmasterd -e critical -l syslog
```

### Command Line Options

```
-c, --configuration  Path to configuration file
-k, --nocleanup      Skip log cleanup on startup
-q, --childlogdir    Child log directory
-l, --logfile        Supervisor log file
-e, --loglevel       Log level (critical, error, warning, info, debug)
-v, --version        Show version and exit
```

### Interactive Commands

Once the supervisor is running, you can use the interactive shell:

```bash
taskmaster> help
┌────────────────────── Available Actions ─────────────────────┐
│ Type 'help <action>'                                         │
└──────────────────────────────────────────────────────────────┘
start       stop        restart     status      
update      quit        exit        shutdown

# Start all processes
taskmaster> start all

# Start specific process
taskmaster> start nginx

# Stop processes
taskmaster> stop worker:*

# Check status
taskmaster> status all
nginx                             RUNNING      pid 1234, uptime 0:05:23
worker:worker_00                  RUNNING      pid 1235, uptime 0:05:20
worker:worker_01                  RUNNING      pid 1236, uptime 0:05:20

# Reload configuration
taskmaster> update

# Exit supervisor
taskmaster> quit
```

### Signal Handling

- `SIGTERM`, `SIGINT`, `SIGQUIT`: Graceful shutdown
- `SIGHUP`: Reload configuration

## 🔄 Process States

- **STOPPED**: Process is not running
- **STARTING**: Process is starting up
- **RUNNING**: Process is running normally  
- **BACKOFF**: Process failed to start, will retry
- **STOPPING**: Process is being stopped
- **EXITED**: Process exited (may restart based on policy)
- **FATAL**: Process failed to start after maximum retries

## 📋 Logging

Taskmaster provides comprehensive logging at multiple levels:

- **CRITICAL**: System-level failures
- **ERROR**: Process failures and errors
- **WARNING**: Unexpected but recoverable events
- **INFO**: Normal operational messages
- **DEBUG**: Detailed debugging information

Logs can be written to:
- File (default)
- Syslog (set `logfile: syslog`)

Child process logs are automatically managed and can be set to:
- `auto`: Automatic file naming in `childlogdir`
- `none`: Discard output
- File path: Specific file location

## 🏗️ Architecture

Taskmaster consists of several key components:

- **Supervisor**: Main orchestrator managing all processes
- **Program Manager**: Manages individual program configurations and their processes
- **Controller**: Interactive command-line interface
- **Logger**: Centralized logging system
- **Config**: Configuration parsing and validation

## 🙏 Acknowledgments

- Inspired by [Supervisor](http://supervisord.org/)
- Built with Go's excellent standard library
- Uses [readline](https://github.com/chzyer/readline) for interactive shell
- Configuration parsing with [mapstructure](https://github.com/mitchellh/mapstructure) and [yaml.v3](https://gopkg.in/yaml.v3)
