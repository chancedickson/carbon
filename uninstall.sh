#!/bin/bash
sudo bash <<EOF

launchctl unload /Library/LaunchDaemons/io.carbon.plist
launchctl stop /Library/LaunchDaemons/io.carbon.plist
rm /Library/LaunchDaemons/io.carbon.plist
rm /usr/local/bin/carbon
rm /etc/carbon.id

EOF
