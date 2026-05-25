#!/bin/sh
echo "Installing netpulse to /usr/local/bin..."
sudo cp -f "$(dirname "$0")/netpulse" /usr/local/bin/netpulse
sudo chmod +x /usr/local/bin/netpulse
echo "Done! Run 'netpulse' from any terminal."