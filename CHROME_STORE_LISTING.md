# Chrome Web Store Listing Content

## Extension Description

**HTR NControl** is a Chrome extension that helps you create step-by-step documentation by automatically recording your browser interactions. Perfect for creating tutorials, training materials, bug reports, and user guides.

### Key Features:
• **Automatic Screenshot Capture** - Takes screenshots as you click, type, and navigate
• **Smart Interaction Tracking** - Records clicks, form inputs, and page navigation automatically
• **Sensitive Data Protection** - Automatically detects and masks passwords, credit card numbers, and other sensitive fields
• **Multiple Export Formats** - Export as JSON (for automation), Markdown (for documentation), or ZIP with embedded images
• **Visual Timeline** - Review your recording step-by-step in an easy-to-use side panel
• **Privacy-First Design** - All data stays on your device. Nothing is sent to external servers

### Perfect For:
• Creating software tutorials and how-to guides
• Documenting workflows and processes
• Training new team members
• Filing detailed bug reports with steps to reproduce
• Building user manuals and knowledge bases

### How It Works:
1. Click the extension icon to start recording
2. Perform your actions naturally - click, type, navigate
3. Stop recording when finished
4. Review your timeline and export in your preferred format

All processing happens locally on your device. Your data never leaves your browser.

---

## Privacy Policy

**Last Updated: January 28, 2026**

### Data Collection and Storage

HTR NControl does **not collect, transmit, or store any user data on external servers**. All extension functionality operates entirely within your browser on your local device.

### What We Store Locally

The extension stores the following data **only on your device** using Chrome's local storage:

- **Recording Sessions**: Screenshots, interaction data (clicks, inputs, navigation), and timestamps
- **User Preferences**: Extension settings and configuration

This data is stored using Chrome's Storage API and remains on your device. You can delete all stored data at any time by:
1. Opening Chrome Settings → Privacy and Security → Site Settings → View permissions and data stored across sites
2. Searching for "HTR NControl"
3. Clicking "Remove" to clear all data

### What We Do NOT Collect

- We do not collect personal information
- We do not track your browsing history beyond active recording sessions
- We do not send any data to external servers or third parties
- We do not use analytics or tracking services
- We do not sell or share any user data

### Sensitive Data Protection

The extension includes built-in protection for sensitive information:
- Automatically detects and masks password fields
- Detects and masks credit card inputs
- Masks other sensitive form fields (SSN, API keys, etc.)
- Sensitive values are replaced with "********" in recordings

### Permissions Usage

See the "Permission Justifications" section below for detailed explanations of why each permission is required.

### Data Deletion

You have full control over your data:
- Delete individual recording sessions from the extension's side panel
- Clear all extension data from Chrome's settings
- Uninstall the extension to remove all stored data

### Changes to This Policy

We may update this privacy policy occasionally. The "Last Updated" date at the top indicates the most recent revision. Continued use of the extension after changes constitutes acceptance of the updated policy.

### Contact

For privacy concerns or questions, contact: github+htrcontrol@mercstudio.com

---

## Permission Justifications

### Single Purpose

**Purpose**: Create step-by-step documentation by recording user interactions with screenshots and exporting them in multiple formats.

**Justification**: The extension serves a single, focused purpose - recording browser interactions to create documentation. All features (screenshot capture, interaction tracking, timeline display, export functionality) directly support this core purpose of creating how-to guides and tutorials.

---

### activeTab Permission

**Justification**: Required to capture screenshots of the current tab's visible content when users perform actions. This permission allows the extension to use `chrome.tabs.captureVisibleTab()` only when the user explicitly starts a recording session. Screenshots are essential for creating visual documentation of the recorded workflow.

**Usage**: 
- Captures screenshots when user clicks elements during an active recording
- Only accesses the active tab when recording is in progress
- Does not access tabs in the background or when recording is stopped

---

### tabs Permission

**Justification**: Required to track page navigation and detect when users navigate to new URLs during a recording session. This allows the extension to create accurate documentation that includes navigation steps (e.g., "Navigate to example.com/login").

**Usage**:
- Listens for tab URL changes to record navigation events
- Detects when new tabs are opened during recording
- Does not read or modify page content
- Does not access tabs when recording is inactive

---

### contextMenus Permission

**Justification**: Enables users to quickly start/stop recordings via right-click context menu. This provides a convenient alternative to clicking the extension icon, improving user experience when creating documentation.

**Usage**:
- Adds "Start Recording" and "Stop Recording" options to the right-click menu
- Menu items only appear when contextually relevant
- No data is collected through context menu interactions

---

### downloads Permission

**Justification**: Required to export recording sessions as downloadable files (JSON, Markdown, or ZIP formats). Users need to download their documentation after creating recordings.

**Usage**:
- Triggers file downloads when user clicks export buttons
- Downloads contain only the user's recorded session data
- Files are generated locally and downloaded directly to the user's device
- No data is uploaded or transmitted

---

### storage Permission

**Justification**: Required to save recording sessions and user preferences locally on the device. All data is stored using Chrome's Storage API and never leaves the user's browser.

**Usage**:
- Saves recording sessions (screenshots, interactions, timestamps) to local storage
- Stores user preferences and extension settings
- All data remains on the user's device
- Users can delete stored data at any time from Chrome settings

---

### scripting Permission

**Justification**: Required to inject content scripts into web pages to detect and record user interactions (clicks, form inputs, element selections). This is essential for the core functionality of tracking user actions during documentation creation.

**Usage**:
- Injects scripts only when recording is active
- Detects click events and form inputs on the page
- Identifies clicked elements to generate accurate selectors
- Scripts are removed when recording stops
- Does not modify page content or functionality
- Does not access page data when recording is inactive

---

### sidePanel Permission

**Justification**: Required to display the extension's main user interface as a side panel in Chrome. The side panel provides a non-intrusive way for users to control recordings and view their timeline without covering the page content they're documenting.

**Usage**:
- Displays the recording control interface (Start/Stop buttons)
- Shows a visual timeline of recorded steps with thumbnails
- Provides export options (JSON, Markdown, ZIP)
- Only visible when user opens the side panel
- Does not automatically open or interfere with browsing

---

### Host Permission (<all_urls>)

**Justification**: Required to inject content scripts into any website the user visits during a recording session. Users need to be able to create documentation for any web application or website, so the extension must be able to track interactions across all domains.

**Usage**:
- Content scripts are specified in the manifest with `matches: ['http://*/*', 'https://*/*']`
- Allows the extension to detect clicks, inputs, and interactions on any page
- Scripts only activate when user starts a recording
- Does not access or modify pages when recording is inactive
- Essential for the core purpose: users cannot document workflows if the extension only works on specific sites
- No data is transmitted to external servers - all processing is local

**Security Notes**:
- Content scripts only listen for user interactions during active recordings
- Sensitive form fields (passwords, credit cards) are automatically masked
- No persistent access to page content - scripts are contextual to recording sessions
- Users have complete control via start/stop recording

---

## Summary for Chrome Web Store Review Team

**Extension Purpose**: Create step-by-step documentation by recording browser interactions with screenshots.

**Data Privacy**: 
- Zero data collection - all data stays on user's device
- No external servers or network requests
- No analytics or tracking
- Built-in sensitive data masking

**Permission Usage**: All permissions are strictly necessary for core functionality:
- Screenshots (activeTab) and navigation tracking (tabs) for documentation
- Local storage (storage) for saving sessions on device
- Content script injection (scripting) for detecting interactions
- Host permissions (<all_urls>) to work on any website user wants to document
- Side panel UI (sidePanel) for non-intrusive control interface
- File export (downloads) for sharing documentation
- User convenience features (contextMenus)

**User Control**: Users have complete control - they explicitly start/stop recordings and can delete all data at any time.
