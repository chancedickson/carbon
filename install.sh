#!/bin/bash
sudo bash <<EOF

cp io.carbon.plist /Library/LaunchDaemons
cp carbon /usr/local/bin
chown root:admin /Library/LaunchDaemons/io.carbon.plist
chown root:admin /usr/local/bin/carbon
launchctl load /Library/LaunchDaemons/io.carbon.plist
launchctl start /Library/LaunchDaemons/io.carbon.plist

EOF
