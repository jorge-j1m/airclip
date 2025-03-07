#!/bin/bash

# Exit on error
set -e

# Check if script is run as root
if [ "$EUID" -ne 0 ]; then
  echo "Please run this script with sudo or as root"
  exit 1
fi

# Get the real username (the user who invoked sudo)
USERNAME=${SUDO_USER:-$(whoami)}
USER_HOME=$(eval echo ~$USERNAME)
USER_ID=$(id -u $USERNAME)

# Copy binary to /usr/local/bin
echo "Enter the path to your compiled binary:"
read -e BINARY_PATH

# Check if the binary exists
if [ ! -f "$BINARY_PATH" ]; then
  echo "Binary not found at $BINARY_PATH"
  exit 1
fi

# Copy the binary
cp "$BINARY_PATH" "/usr/local/bin/airclip"
chmod +x "/usr/local/bin/airclip"

# Detect if system is using Wayland or X11
WAYLAND_SESSION=""
if [ -n "$SUDO_USER" ]; then
  # Get the session type from the user's environment
  WAYLAND_SESSION=$(su - $USERNAME -c 'echo $WAYLAND_DISPLAY')
fi

# Find the appropriate XAUTHORITY file
XAUTHORITY_PATH="$USER_HOME/.Xauthority"  # Default X11 path
if [ -n "$WAYLAND_SESSION" ] || [ -d "/run/user/$USER_ID" ]; then
  # System likely using Wayland
  # Create a script to find the right Xauthority file at runtime
  cat > /usr/local/bin/airclip-launcher.sh << EOF
#!/bin/bash
export DISPLAY=:0
# Find the Xauthority file for Wayland (Xwayland)
for auth_file in /run/user/$USER_ID/*Xwayland*auth* /run/user/$USER_ID/.mutter-Xwaylandauth.*; do
  if [ -e "\$auth_file" ]; then
    export XAUTHORITY="\$auth_file"
    break
  fi
done

# If no Wayland auth file found, try the default
if [ -z "\$XAUTHORITY" ] && [ -e "$USER_HOME/.Xauthority" ]; then
  export XAUTHORITY="$USER_HOME/.Xauthority"
fi

# Run airclip with all arguments passed to this script
exec /usr/local/bin/airclip "\$@"
EOF
  chmod +x /usr/local/bin/airclip-launcher.sh
  EXEC_PATH="/usr/local/bin/airclip-launcher.sh"
  echo "Detected Wayland session, using wrapper script for compatibility"
else
  # Using regular X11
  EXEC_PATH="/usr/local/bin/airclip"
  echo "Detected X11 session, using standard configuration"
fi

# Create systemd service file
cat > /etc/systemd/system/airclip.service << EOF
[Unit]
Description=Airclip Service
After=network.target graphical-session.target

[Service]
Type=simple
User=$USERNAME
ExecStart=$EXEC_PATH
Environment=DISPLAY=:0
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

# Configure systemd
systemctl daemon-reload
systemctl enable airclip.service
systemctl start airclip.service

echo "Airclip has been installed and started."
echo "Service status:"
systemctl status airclip.service
