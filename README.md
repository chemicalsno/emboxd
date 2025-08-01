<div align="center">
  <a href="https://emby.media/"><img src="https://img.shields.io/badge/Emby-52b54b?logo=emby&logoColor=white"/></a>
  <a href="https://github.com/computer-geek64/emboxd/releases/latest"><img src="https://img.shields.io/github/v/release/computer-geek64/emboxd"/></a>
  <a href="https://github.com/search?q=repo%3Acomputer-geek64%2Femboxd++language%3AGo&type=code"><img src="https://img.shields.io/github/languages/top/computer-geek64/emboxd"/></a>
  <a href="https://github.com/computer-geek64/emboxd/issues?q=is%3Aissue%20state%3Aopen%20label%3Abug"><img src="https://img.shields.io/github/issues/computer-geek64/emboxd/bug"/></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/computer-geek64/emboxd"/></a>
  <a href="https://github.com/computer-geek64/emboxd/forks"><img src="https://img.shields.io/github/forks/computer-geek64/emboxd"/></a>
  <a href="https://github.com/computer-geek64/emboxd/stargazers"><img src="https://img.shields.io/github/stars/computer-geek64/emboxd"/></a>

  <h1>EmBoxd</h1>

  <h4>Live sync server for Letterboxd users with self-hosted media platforms</h4>
</div>


## Table of Contents

- [Table of Contents](#table-of-contents)
- [About](#about)
- [Installation](#installation)
  - [Binary](#binary)
  - [Docker](#docker)
  - [Docker Compose](#docker-compose)
  - [Unraid Deployment](#unraid-deployment)
  - [Backup and Persistence](#backup-and-persistence)
- [Usage](#usage)
  - [Configuration](#configuration)
  - [Plex Setup](#plex-setup)
  - [Running](#running)
  - [API Endpoints](#api-endpoints)
  - [Advanced Features](#advanced-features)
    - [Enhanced Logging](#enhanced-logging)
    - [Error Handling \& Recovery](#error-handling--recovery)
    - [Event History](#event-history)
- [Contributors](#contributors)
- [License](#license)


## About

EmBoxd provides live integration with Letterboxd for users of self-hosted media servers.
It tracks watch activity on the media server and synchronizes Letterboxd user data to match.
Changes to a movie's played status are reflected in the user's watched films, and movies that are fully played are logged in the user's diary.

The following media servers are currently supported or have planned support:

- [X] Emby
- [ ] Jellyfin [#4](https://github.com/computer-geek64/emboxd/issues/4)
- [ ] Plex [#6](https://github.com/computer-geek64/emboxd/issues/6)
- [X] Plex (webhook, Plex Pass required)


## Installation

EmBoxd can either be setup and used as a binary or Docker image

### Binary

Building a binary from source requires the Go runtime

1. Clone repository:

```sh
git clone https://github.com/computer-geek64/emboxd.git --depth=1
cd emboxd/
```

2. Install Playwright browsers and OS dependencies:

```sh
go install github.com/playwright-community/playwright-go/cmd/playwright
playwright install --with-deps
```

3. Build and install binary (to GOPATH)

```sh
go install .
```

### Docker

Pull from GitHub container registry:

```sh
docker pull ghcr.io/computer-geek64/emboxd:latest
```

Or build image from source:

```sh
git clone https://github.com/computer-geek64/emboxd.git --depth=1
docker build -t emboxd:latest emboxd/
```

### Docker Compose

For easier deployment, you can use Docker Compose:

```sh
git clone https://github.com/computer-geek64/emboxd.git --depth=1
cd emboxd
docker-compose up -d
```

For better build performance, you can enable Compose to delegate builds to BuildKit:

```sh
export COMPOSE_BAKE=true
docker-compose up -d
```

### Unraid Deployment

To run EmBoxd on an Unraid server:

1. Open the Unraid web interface and navigate to the "Docker" tab
2. Click "Add Container"
3. Fill in the following details:
   - **Repository**: `yourusername/emboxd:latest` (or build your own image)
   - **Network Type**: Bridge
   - **Port**: Map container port 80 to a host port (e.g., 8080)
   - **Volumes**:
     - Host path: `/mnt/user/appdata/emboxd/config` → Container path: `/config`
     - Host path: `/mnt/user/appdata/emboxd/logs` → Container path: `/logs`
     - Host path: `/mnt/user/appdata/emboxd/data` → Container path: `/data`
   - **Variables**:
     - TZ=Your timezone (e.g., America/New_York)
4. Click "Apply"

Alternatively, you can use the Community Applications plugin and import the docker-compose.yml file.

### Backup and Persistence

EmBoxd stores all persistent data in the following directories:

- `/config` - Configuration files, including `config.yaml`
- `/logs` - Log files for troubleshooting and audit trails
- `/data` - Application data including cached information

When running on Unraid, these directories are mapped to your array storage and should be included in your regular backup strategy. You can back them up by:

1. Including the `/mnt/user/appdata/emboxd/` directory in your Unraid backup jobs
2. Manually copying these directories to a safe location
3. Using the Unraid backup plugins like "CA Backup / Restore Appdata"

## Usage

### Configuration

The YAML configuration file describes how to link Letterboxd accounts with media server users.
The format should follow the example [`config.yaml`](config.yaml) in the repository root.

Supported media servers need to send webhook notifications for all (relevant) users to the EmBoxd server API.

Emby should send the following notifications to `/emby/webhook`:

- [X] Playback
  - [X] Start
  - [X] Pause
  - [X] Unpause
  - [X] Stop
- [X] Users
  - [X] Mark Played
  - [X] Mark Unplayed

### Plex Setup

Setting up Plex integration requires a Plex Pass subscription:

1. Log in to your Plex server web interface
2. Navigate to **Settings** › **Network**
3. Scroll down to the **Webhooks** section
4. Click **Add Webhook**
5. Enter your EmBoxd server URL followed by `/plex/webhook` (e.g., `http://your-emboxd-server/plex/webhook`)
6. Click **Save Changes**

EmBoxd handles the following Plex webhook events:
- `media.play` - When playback starts
- `media.pause` - When playback is paused
- `media.resume` - When playback resumes after being paused
- `media.stop` - When playback stops
- `media.scrobble` - When a movie is marked as played (typically at 90% watched)

In your `config.yaml`, map each Plex user to their corresponding Letterboxd account. You can configure users in one of three ways:

```yaml
users:
  # Option 1: Using only username (display name)
  - letterboxd:
      username: letterboxd_username1
      password: "${LETTERBOXD_PASSWORD1}"
    plex:
      username: Plex Display Name  # The Account.title from webhook

  # Option 2: Using only account ID (recommended for reliability)
  - letterboxd:
      username: letterboxd_username2
      password: "${LETTERBOXD_PASSWORD2}"
    plex:
      id: "12345"  # The stable Account.id from webhook

  # Option 3: Using both (ID takes precedence, username used as fallback)
  - letterboxd:
      username: letterboxd_username3
      password: "${LETTERBOXD_PASSWORD3}"
    plex:
      username: Plex Display Name
      id: "67890"
```

For maximum reliability, it's recommended to use the `id` field which contains the stable Plex account identifier. Display names can change, but the account ID remains constant.

If you don't know a user's Plex Account ID, you can first set up with just the username, then check your EmBoxd logs after a webhook is received to see the Account ID in the log messages.

### Running

Running EmBoxd starts the server and binds with port 80.
The following command-line options are available:

- `-c`, `--config` - Path to configuration file (default: "config/config.yaml")
- `-v`, `--verbose` - Enable debug logging
- `--history-size` - Maximum number of events to keep in history (default: 100)
- `--log-dir` - Directory for log files (empty for stdout only)
- `--log-json` - Output logs in JSON format

### API Endpoints

EmBoxd provides the following API endpoints:

- `/` - Welcome message
- `/health` - Health check endpoint that provides:
  - Overall service status
  - Server uptime
  - Status of all Letterboxd connections
- `/events` - Event history endpoint that provides:
  - Recent events processed by the service
  - Status of each event (success, error)
  - Details about media, user, and timing
- `/emby/webhook` - Webhook receiver for Emby
- `/plex/webhook` - Webhook receiver for Plex

### Advanced Features

EmBoxd includes several advanced features for improved reliability:

#### Enhanced Logging
- Structured logs with detailed context
- Multiple log levels (info, debug, warn, error)
- Optional JSON output format for log aggregation
- Log file rotation with configurable retention

#### Error Handling & Recovery
- Robust error classification system
- Automatic retry mechanism with exponential backoff
- Graceful recovery from temporary failures
- Detailed error reporting in logs

#### Event History
- In-memory storage of recent events
- API endpoint to retrieve event history
- Detailed status tracking for monitoring

When running with Docker, the image expects the configuration file at `/config/config.yaml`.
It can be bind-mounted to the container or stored in a volume.

```sh
docker run --name=emboxd --restart=unless-stopped -v config.yaml:/config/config.yaml:ro -p 80:80 ghcr.io/computer-geek64/emboxd:latest
```


## Contributors

<a href="https://github.com/computer-geek64/emboxd/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=computer-geek64/emboxd"/>
</a>


## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
