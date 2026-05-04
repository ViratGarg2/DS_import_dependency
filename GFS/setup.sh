#!/bin/bash
# GFS Setup Helper Script
# Provides easy switching between deployment modes

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Functions
print_header() {
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}→ $1${NC}"
}

show_menu() {
    echo ""
    echo "GFS Deployment Mode Selection:"
    echo "  1) Docker Mode (client inside container)"
    echo "  2) External Mode (client on host/remote)"
    echo "  3) Show current configuration"
    echo "  4) Clean & restart all services"
    echo "  5) View logs"
    echo "  6) Exit"
    echo ""
    read -p "Select option (1-6): " choice
}

setup_docker_mode() {
    print_header "Setting Up Docker Mode"
    
    if [ -f ".env" ]; then
        print_info "Backing up current .env to .env.backup"
        cp .env .env.backup
    fi
    
    print_info "Copying .env.docker to .env"
    cp .env.docker .env
    
    print_success "Docker mode configured"
    print_info "To start services: docker compose up -d --build"
    print_info "To use client: docker exec -it gfs-client /usr/local/bin/gfs-client --config /app/configs/docker/client-config.yml"
}

setup_external_mode() {
    print_header "Setting Up External Mode"
    
    # Get current IP
    DEFAULT_IP=$(hostname -I | awk '{print $1}')
    
    print_info "Your server IP: $DEFAULT_IP"
    read -p "Enter server IP (or press Enter for $DEFAULT_IP): " SERVER_IP
    SERVER_IP=${SERVER_IP:-$DEFAULT_IP}
    
    if [ -f ".env" ]; then
        print_info "Backing up current .env to .env.backup"
        cp .env .env.backup
    fi
    
    print_info "Copying .env.external to .env"
    cp .env.external .env
    
    print_info "Updating IP address in .env"
    sed -i "s/10.236.97.159/$SERVER_IP/g" .env
    
    # Also update the external client config
    if [ -f "configs/external/client-config.yml" ]; then
        sed -i "s/10.236.97.159/$SERVER_IP/g" configs/external/client-config.yml
    fi
    
    print_success "External mode configured with IP: $SERVER_IP"
    print_info "To start services: docker compose up -d --build"
    print_info "To build client on host: export PATH=\$HOME/go/bin:\$PATH && make build-client"
    print_info "To use client: ./bin/gfs-client --config configs/external/client-config.yml"
}

show_config() {
    print_header "Current Configuration"
    if [ -f ".env" ]; then
        echo "Mode: $(grep '^GFS_MODE=' .env || echo 'NOT SET')"
        echo "Chunk1 Host: $(grep '^GFS_CHUNK1_HOST=' .env || echo 'NOT SET')"
        echo "Chunk2 Host: $(grep '^GFS_CHUNK2_HOST=' .env || echo 'NOT SET')"
        echo "Master Host: $(grep '^GFS_MASTER_HOST=' .env || echo 'NOT SET')"
        echo ""
        echo "Services status:"
        docker compose ps --no-trunc 2>/dev/null || echo "Docker services not running"
    else
        print_error "No .env file found"
    fi
}

clean_and_restart() {
    print_header "Clean Restart"
    read -p "This will remove all containers and volumes. Continue? (y/N): " confirm
    if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
        print_info "Cancelled"
        return
    fi
    
    print_info "Stopping and removing containers..."
    docker compose down -v 2>/dev/null || true
    
    print_info "Rebuilding and starting services..."
    docker compose up -d --build
    
    print_success "Services restarted"
    sleep 5
    docker compose ps
}

view_logs() {
    echo ""
    echo "View logs from:"
    echo "  1) Master"
    echo "  2) Chunk1"
    echo "  3) Chunk2"
    echo "  4) Client"
    echo "  0) Cancel"
    read -p "Select (0-4): " log_choice
    
    case $log_choice in
        1) docker logs gfs-master -n 50 ;;
        2) docker logs gfs-chunk1 -n 50 ;;
        3) docker logs gfs-chunk2 -n 50 ;;
        4) docker logs gfs-client -n 50 ;;
        0) return ;;
        *) print_error "Invalid option" ;;
    esac
}

# Main loop
while true; do
    show_menu
    
    case $choice in
        1)
            setup_docker_mode
            ;;
        2)
            setup_external_mode
            ;;
        3)
            show_config
            ;;
        4)
            clean_and_restart
            ;;
        5)
            view_logs
            ;;
        6)
            print_info "Exiting"
            exit 0
            ;;
        *)
            print_error "Invalid option. Please select 1-6."
            ;;
    esac
    
    read -p "Press Enter to continue..."
done
