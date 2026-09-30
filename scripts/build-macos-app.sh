#!/bin/bash
# Build macOS .app bundle containing the single Clawy executable.

set -e

EXECUTABLE=$1
APP_VERSION_ARG=$2

if [ -z "$EXECUTABLE" ]; then
    echo "Usage: $0 <platform-arch> [version]"
    exit 1
fi

EXECUTABLE="clawy-${EXECUTABLE}"
echo "executable: $EXECUTABLE"

APP_NAME="Clawy"
APP_PATH="./build/${APP_NAME}.app"
APP_CONTENTS="${APP_PATH}/Contents"
APP_MACOS="${APP_CONTENTS}/MacOS"
APP_RESOURCES="${APP_CONTENTS}/Resources"
APP_EXECUTABLE="clawy"
ICON_SOURCE="./scripts/icon.icns"

# Clean up existing .app
if [ -d "$APP_PATH" ]; then
    echo "Removing existing ${APP_PATH}"
    rm -rf "$APP_PATH"
fi

# Create directory structure
echo "Creating .app bundle structure..."
mkdir -p "$APP_MACOS"
mkdir -p "$APP_RESOURCES"

# Copy executable
echo "Copying executable..."
if [ -f "./build/${EXECUTABLE}" ]; then
    cp "./build/${EXECUTABLE}" "${APP_MACOS}/${APP_EXECUTABLE}"
else
    echo "Error: ./build/${EXECUTABLE} not found. Please build the main file first."
    echo "Run: make build"
    exit 1
fi
chmod +x "${APP_MACOS}/"*

# Create Info.plist
echo "Creating Info.plist..."
cat > "${APP_CONTENTS}/Info.plist" << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>clawy</string>
    <key>CFBundleIdentifier</key>
    <string>com.clawy.app</string>
    <key>CFBundleName</key>
    <string>Clawy</string>
    <key>CFBundleDisplayName</key>
    <string>Clawy</string>
    <key>CFBundleIconFile</key>
    <string>icon.icns</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0</string>
    <key>CFBundleVersion</key>
    <string>1</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSSupportsAutomaticGraphicsSwitching</key>
    <true/>
    <key>LSUIElement</key>
    <true/>
    <key>LSMinimumSystemVersion</key>
    <string>10.11</string>
</dict>
</plist>
EOF

#sips -z 128 128 "$ICON_SOURCE" --out "${ICONSET_PATH}/icon_128x128.png" > /dev/null 2>&1
#
## Create icns file
#iconutil -c icns "$ICONSET_PATH" -o "$ICON_OUTPUT" 2>/dev/null || {
#    echo "Warning: iconutil failed"
#}

cp $ICON_SOURCE "${APP_RESOURCES}/icon.icns"

# Inject the real version into Info.plist. Prefers the value passed by the
# caller (e.g. "v1.2.3" from CI); falls back to git describe for local builds.
if [ -n "$APP_VERSION_ARG" ]; then
    APP_VERSION="${APP_VERSION_ARG#v}"
else
    APP_VERSION="$(git describe --tags --always --dirty 2>/dev/null | sed -e 's/^v//')"
fi
if [ -z "$APP_VERSION" ]; then
    APP_VERSION="0.0.0-dev"
fi

echo "Setting bundle version to ${APP_VERSION}..."
if [ -x "/usr/libexec/PlistBuddy" ]; then
    /usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString ${APP_VERSION}" "${APP_CONTENTS}/Info.plist"
    /usr/libexec/PlistBuddy -c "Set :CFBundleVersion ${APP_VERSION}" "${APP_CONTENTS}/Info.plist"
else
    sed -i '' \
        -e "s|<string>1\.0</string>|<string>${APP_VERSION}</string>|" \
        -e "s|<string>1</string>|<string>${APP_VERSION}</string>|" \
        "${APP_CONTENTS}/Info.plist"
fi

echo ""
echo "=========================================="
echo "Successfully created: ${APP_PATH}"
echo "=========================================="
echo ""
echo "To launch Clawy:"
echo "  1. Double-click ${APP_NAME}.app in Finder"
echo "  2. Or use: open ${APP_PATH}"
echo ""
echo "The same Clawy executable opens the web console in your browser."
echo ""
