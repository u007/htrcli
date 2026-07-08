#!/usr/bin/env bash

set -e

echo "Extracting public key from CRX_PRIVATE_KEY secret..."
echo ""
echo "Note: gh CLI doesn't support 'secret get' - you'll need to:"
echo "1. Go to https://github.com/AStevensTaylor/htrncontrol/settings/secrets/actions"
echo "2. Copy the CRX_PRIVATE_KEY value"
echo "3. Save it to a temporary file (e.g., /tmp/key.pem)"
echo "4. Run: openssl rsa -in /tmp/key.pem -pubout"
echo "5. Delete the temporary file: rm /tmp/key.pem"
echo ""
echo "Then copy the public key output and:"
echo "1. Go to https://chrome.google.com/webstore/devconsole"
echo "2. Select your extension"
echo "3. Navigate to the Package tab"
echo "4. Find the 'Verified CRX Uploads' section"
echo "5. Click 'Opt In'"
echo "6. Paste the public key when prompted"
echo ""
echo "This will protect your extension updates from unauthorized modifications."
