# ssht

Interactive SSH session manager with AI keystroke control and background sessions.

## Features

- **Interactive TUI** — Host list with fuzzy search, quick connect
- **Auto Profile** — First connection auto-saves host to profile, imports `~/.ssh/config`
- **Background Sessions** — tmux-like detach/attach with persistent connections
- **AI Keystroke Control** — Send commands and key sequences to background sessions programmatically
- **SSH Key Auto-Deploy** — On first login, offers to generate and deploy SSH keys
- **Multi-Server Execution** — Run commands across multiple hosts in parallel
- **Encrypted Credentials** — Passwords stored with AES-GCM encryption (machine-bound key)

## Install

```bash
go install github.com/chonamkyu/ssht@latest
```

Or build from source:

```bash
git clone https://github.com/chonamkyu/ssht.git
cd ssht
go build -o ssht .
```

## Usage

### Connect to a host

```bash
ssht connect user@hostname
ssht connect user@hostname:2222
ssht connect user@hostname -p "password"
ssht connect myserver -b  # background session
```

First connection automatically registers the host and offers SSH key deployment.

### Interactive TUI

```bash
ssht  # launches interactive host selector
```

Keys: `enter` connect, `b` background, `s` sessions, `q` quit

### Session Management

```bash
ssht sessions              # list active sessions
ssht attach 1              # reattach to session
ssht sessions kill 1       # kill a session
ssht sessions kill --all   # kill all sessions
```

Detach from a session with `Ctrl+B` then `d`.

### AI Keystroke Control

Send commands and keystrokes to background sessions:

```bash
ssht send 1 --cmd "ls -la"         # execute command (auto-enter)
ssht send 1 --key "ctrl+c"         # send Ctrl+C
ssht send 1 --key "ctrl+z"         # send Ctrl+Z
ssht send 1 --key "up,up,enter"    # key sequence
ssht read 1                         # read terminal buffer
ssht read 1 --follow                # stream output
```

Supported keys: `ctrl+a-z`, `enter`, `tab`, `escape`, `up`, `down`, `left`, `right`, `home`, `end`, `delete`, `pgup`, `pgdown`, `f1-f12`, `backspace`

### Multi-Server Execution

```bash
ssht exec "uptime" -h server1,server2
ssht exec "df -h" --tag prod
ssht exec "systemctl status nginx" --tag web --json
```

### Host Management

```bash
ssht hosts                                          # list all hosts
ssht hosts add --name myserver --host 10.0.1.5 --user root
ssht hosts remove myserver
```

## Configuration

Config is stored at `~/.ssht/config.yaml`:

```yaml
hosts:
  - name: prod-web-1
    host: 10.0.1.10
    port: 22
    user: deploy
    key: ~/.ssh/id_ed25519
    tags: [prod, web]
```

- Hosts from `~/.ssh/config` are auto-imported (read-only)
- Passwords are AES-GCM encrypted with a machine-bound key (`~/.ssht/key`)

## Architecture

```
ssht
├── cmd/              CLI commands (cobra)
├── internal/
│   ├── config/       Host profiles, SSH config parser, encryption
│   ├── ssh/          SSH client, key generation/deployment
│   ├── keys/         Keystroke parsing (ctrl sequences, special keys)
│   ├── session/      Background session daemon (unix socket IPC)
│   ├── multi/        Parallel multi-host execution
│   └── tui/          Interactive terminal UI (bubbletea)
└── main.go
```

## License

MIT
