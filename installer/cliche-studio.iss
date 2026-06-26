; Cliche Studio — Windows installer.
; Build with Inno Setup 6:  ISCC installer\cliche-studio.iss
; Produces dist\ClicheStudioSetup.exe — a double-click installer that drops the
; CLI + the WebView2 desktop shell and adds Start-Menu / Desktop shortcuts.
;
; Prerequisites built into dist\ first (see installer\README.md or the Makefile):
;   dist\cliche.exe          (the zero-dep CLI, with the Studio UI embedded)
;   dist\cliche-studio.exe   (the WebView2 desktop shell)

#define AppName "Cliche Studio"
#define AppVersion "0.1.0"
#define Publisher "mholovetskyi"

[Setup]
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#Publisher}
DefaultDirName={autopf}\Cliche
DefaultGroupName=Cliche
DisableProgramGroupPage=yes
OutputDir=..\dist
OutputBaseFilename=ClicheStudioSetup
Compression=lzma2
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
WizardStyle=modern
UninstallDisplayIcon={app}\cliche-studio.exe
; The Microsoft Edge WebView2 runtime ships with Windows 10/11. If a target lacks
; it, install it from https://developer.microsoft.com/microsoft-edge/webview2/.

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Shortcuts:"

[Files]
Source: "..\dist\cliche.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\cliche-studio.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\Cliche Studio"; Filename: "{app}\cliche-studio.exe"
Name: "{group}\Uninstall Cliche Studio"; Filename: "{uninstallexe}"
Name: "{autodesktop}\Cliche Studio"; Filename: "{app}\cliche-studio.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\cliche-studio.exe"; Description: "Launch Cliche Studio now"; Flags: nowait postinstall skipifsilent
