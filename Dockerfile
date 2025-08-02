FROM golang:1.22.9

WORKDIR /code

# Copy dependency files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Install Playwright and its dependencies
RUN go install github.com/playwright-community/playwright-go/cmd/playwright@latest
ENV PLAYWRIGHT_BROWSERS_PATH=/root/.cache/ms-playwright

# Install required system dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates fonts-liberation libappindicator3-1 \
    libasound2 libatk-bridge2.0-0 libatk1.0-0 libc6 \
    libcairo2 libcups2 libdbus-1-3 libexpat1 libfontconfig1 \
    libgbm1 libgcc1 libglib2.0-0 libgtk-3-0 libnspr4 libnss3 \
    libpango-1.0-0 libpangocairo-1.0-0 libstdc++6 libx11-6 \
    libx11-xcb1 libxcb1 libxcomposite1 libxcursor1 libxdamage1 \
    libxext6 libxfixes3 libxi6 libxrandr2 libxrender1 libxss1 \
    libxtst6 lsb-release wget xdg-utils nodejs npm && \
    rm -rf /var/lib/apt/lists/*

# Install Firefox browser and drivers for Playwright
# Following recommended approach from official docs: https://github.com/playwright-community/playwright-go
RUN go install github.com/playwright-community/playwright-go/cmd/playwright@v0.4902.0 && \
    playwright install firefox --with-deps && \
    # Additional redundant driver installation for version matching
    cd /tmp && \
    # Install specific matching playwright version based on go.mod (v0.4902.0 uses playwright v1.49.1)
    npm init -y && \
    npm install playwright@1.49.1 && \
    npx playwright install && \
    # Create directories and symlinks to ensure proper driver detection
    mkdir -p /root/.cache/ms-playwright && \
    # Set up proper symlinks and file permissions
    chmod -R 777 /root/.cache/ms-playwright && \
    # Create symlinks in alternate paths that playwright-go might check
    mkdir -p /go/pkg/mod/github.com/playwright-community && \
    ln -sf /root/.cache/ms-playwright /go/pkg/mod/github.com/playwright-community/playwright-drivers && \
    # Show results for debugging
    ls -la /root/.cache/ms-playwright

# Copy source code
COPY . .

# Build the application
RUN go install .

# Copy entrypoint script
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Create directories
RUN mkdir -p /logs /data /config && \
    chmod -R 755 /logs && \
    touch /logs/emboxd.log && \
    chmod 644 /logs/emboxd.log

# Set the entrypoint and default command
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["-c", "/config/config.yaml"]
