# Airclip

A lightweight notification and clipboard sharing system for local networks, enabling seamless text sharing between iOS and Linux devices.

## Motivation

Airclip was created to solve a specific, common problem: securely sharing passwords and sensitive text between an iPhone and a Linux desktop. When setting up a new Linux system, constantly typing complex passwords from a password manager on your phone becomes tedious and error-prone.

**Key benefits:**
- Share passwords and sensitive text from iOS to Linux with a single tap
- Text automatically appears as a notification on your desktop
- Text is automatically copied to your clipboard, ready to paste
- No need to email yourself passwords or manually retype complex credentials
- Works entirely within your local network (no cloud services or internet dependency)
- Zero configuration required on the iOS side (just a simple Shortcut)

## Security By Design

Airclip was built with local-network security as a core principle, which is essential when sharing sensitive information like passwords:

- **Local network only:** The server validates source IPs and rejects connections from non-local networks
- **Authentication:** Simple token-based authentication for requests
- **No cloud dependencies:** Everything runs on your devices within your home network
- **Transparent logging:** All activities are logged with timestamps for auditing

## How It Works

1. The Airclip server runs on your Linux desktop, listening for HTTP requests
2. When you select text (like a password) on your iPhone, you share it via a simple iOS Shortcut
3. The text appears instantly as a notification on your Linux desktop
4. The text is automatically copied to your clipboard, ready to paste into login forms or terminal

## Installation

### Prerequisites

- A Linux/Ubuntu system with `notify-send` and `xclip`
- iPhone with the Shortcuts app
- Both devices on the same local network

### Server Setup

1. Install required packages:

```bash
sudo apt-get update
sudo apt-get install -y xclip golang-go
```

2. Build the Airclip server:

```bash
go build -o airclip main.go
```

3. Run the server:

```bash
./airclip
```

The server will display all available local IP addresses. Note the IP address you want to use.

### Setting up Airclip as a Service

To have Airclip start automatically at boot time:

1. The repository includes an installation script (`install-airclip.sh`). Make it executable:

```bash
chmod +x install-airclip.sh
```

2. Run it with sudo:

```bash
sudo ./install-airclip.sh
```

4. When prompted, provide the path to your compiled Airclip binary.

That's it! Airclip will now start automatically whenever your system boots.

#### Managing the Airclip Service

- Check status: `systemctl status airclip.service`
- Start manually: `sudo systemctl start airclip.service`
- Stop service: `sudo systemctl stop airclip.service`
- Restart service: `sudo systemctl restart airclip.service`
- View logs: `journalctl -u airclip.service`

### iOS Shortcut Setup

1. Open the Shortcuts app on your iPhone
2. Create a new shortcut
3. Add the "URL" action with your server's local IP: `http://10.0.X.Y:9123/notify`
4. Add the "Get Contents of URL" action with:
   - Method: POST
   - Headers:
     - Authorization: Bearer local-use-only
     - Content-Type: text/plain
   - Request Body: Shortcut Input
5. Optional: Add a "Show Notification" action for confirmation
6. Name your shortcut "Airclip"
7. Save, enable in Share Sheet, and set type to "Text"

## Usage

1. On your iOS device, select any text (such as a password from your password manager)
2. Tap the Share button
3. Select your "Airclip" shortcut
4. The text immediately appears as a notification on your Linux desktop and is copied to the clipboard, ready to paste into login forms

## Configuration Options

The server supports several command-line options:

```
--port      HTTP port to listen on (default: 9123)
--listen    IP address to listen on (default: 0.0.0.0)
--token     Authentication token (default: local-use-only)
--local-only  Restrict to local network connections only (default: true)
--cors      Enable CORS for cross-origin requests (default: true)
--logdir    Directory to store log files (default: /tmp)
```

## Troubleshooting

- **Server won't start:** Check permissions and port availability
- **iPhone can't connect:** Verify both devices are on the same network
- **No notifications:** Ensure notify-send is working (`notify-send "Test"`)
- **Clipboard not working:** Verify xclip installation (`xclip -version`)
- **Can't find logs:** Look in `/tmp` for timestamped log files (format: `airclip_YYYY-MM-DD_HH-MM-SS.log`)
- **Service won't start:** Check the service logs with `journalctl -u airclip.service`

## Use Cases

- **Share passwords** from your password manager to login forms or terminal
- **Transfer 2FA verification codes** without manual retyping
- **Send SSH keys** or other configuration snippets while setting up a new system
- **Copy URLs** from your phone to your desktop browser
- **Transfer API keys** or tokens securely within your network
- **Share configuration snippets** while following tutorials on your phone

## License

Airclip is distributed under the MIT license. See LICENSE for more information.

## Contributing

Contributions to Airclip are welcome! Please feel free to submit a Pull Request
