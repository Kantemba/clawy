; Inno Setup script for the Clawy Windows installer.
;
; Build (from repository root, after the binaries have been produced):
;   ISCC.exe /DMyAppVersion=v1.2.3 scripts\setup.iss
;
; Expected inputs (paths relative to scripts/):
;   ..\build\clawy.exe            single binary (CLI, web console, and gateway)
;   ..\clients\web\backend\icon.ico application icon

#define MyAppName "Clawy"
#define MyAppPublisher "Kantemba"
#define MyAppURL "https://github.com/Kantemba/clawy"
#define MyAppExeName "clawy.exe"

; Allow the version to be injected by the CI via /DMyAppVersion=x.y.z,
; falling back to a clearly-marked development value for local builds.
#ifndef MyAppVersion
#define MyAppVersion "0.0.0-dev"
#endif

[Setup]
; NOTE: The value of AppId uniquely identifies this application. Do not use the same AppId value in installers for other applications.
AppId={{C8A1B4E7-D5F9-4C2A-8A6E-5F4D3C2A1B0E}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} {#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
DefaultDirName={autopf}\Clawy
DefaultGroupName={#MyAppName}
UninstallDisplayName={#MyAppName}
UninstallDisplayIcon={app}\icon.ico
; "ArchitecturesAllowed=x64compatible" specifies that Setup cannot run
; on anything but x64 and Windows 11 on Arm.
ArchitecturesAllowed=x64compatible
; "ArchitecturesInstallIn64BitMode=x64compatible" requests that the
; install be done in "64-bit mode" on x64 or Windows 11 on Arm,
; meaning it should use the native 64-bit Program Files directory and
; the 64-bit view of the registry.
ArchitecturesInstallIn64BitMode=x64compatible
DisableProgramGroupPage=yes
; PrivilegesRequired=lowest keeps {autopf} under %LOCALAPPDATA%\Programs,
; so the installer never requires admin rights.
PrivilegesRequired=lowest
OutputDir=..\build
OutputBaseFilename=ClawySetup-{#MyAppVersion}
Compression=lzma
SolidCompression=yes
WizardStyle=modern
SetupIconFile=..\clients\web\backend\icon.ico

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "..\build\clawy.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\clients\web\backend\icon.ico"; DestDir: "{app}"; Flags: ignoreversion
; NOTE: Don't use "Flags: ignoreversion" on any shared system files

; Remove the obsolete companion executable when upgrading an old install.
[InstallDelete]
Type: files; Name: "{app}\clawy-launcher.exe"

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Parameters: "start"; WorkingDir: "{app}"; IconFilename: "{app}\icon.ico"
Name: "{group}\Uninstall {#MyAppName}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Parameters: "start"; WorkingDir: "{app}"; Tasks: desktopicon; IconFilename: "{app}\icon.ico"

[Run]
Filename:"{app}\{#MyAppExeName}"; Parameters: "start"; WorkingDir: "{app}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent
