; Cliché Studio — Windows installer.
; Build with Inno Setup 6:  ISCC installer\cliche-studio.iss
; Produces dist\ClicheStudioSetup.exe — a double-click installer that drops the
; CLI + the WebView2 desktop shell, ensures the WebView2 runtime is present, and
; adds Start-Menu / Desktop shortcuts.
;
; Per-user install (no admin/UAC prompt) — matches how VS Code / Discord install.
;
; Prerequisites in this folder + dist\ first (see installer\README.md / Makefile):
;   dist\cliche.exe                    (the zero-dep CLI, with the Studio UI embedded)
;   dist\cliche-studio.exe             (the WebView2 desktop shell, logo icon embedded)
;   installer\MicrosoftEdgeWebview2Setup.exe   (the Evergreen bootstrapper, ~1.7 MB)
;   assets\logo.ico                    (the installer + app icon)
;
; NOTE: this installer is UNSIGNED. Downloaded from a browser it carries the
; Mark-of-the-Web, so SmartScreen shows "Windows protected your PC — Unknown
; publisher"; users click "More info -> Run anyway". Sign with an OV/EV
; code-signing cert (add a SignTool directive here) before wide public release.

#define AppName "Cliché Studio"
#define AppVersion "0.1.0"
#define Publisher "mholovetskyi"

[Setup]
; A stable AppId (held constant across releases) gives clean in-place upgrades.
AppId={{B7E9B1A2-3C4D-4E5F-9A8B-1C2D3E4F5061}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher={#Publisher}
VersionInfoVersion={#AppVersion}
DefaultDirName={localappdata}\Programs\Cliche
DisableProgramGroupPage=yes
; Per-user: no elevation, installs for the current user only.
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
OutputDir=..\dist
OutputBaseFilename=ClicheStudioSetup
Compression=lzma2
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
WizardStyle=modern
SetupIconFile=..\assets\logo.ico
UninstallDisplayIcon={app}\cliche-studio.exe
ChangesEnvironment=yes

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Shortcuts:"
Name: "addtopath"; Description: "Add the cliche command to PATH (use it in a terminal)"; GroupDescription: "Advanced:"; Flags: unchecked

[Files]
Source: "..\dist\cliche.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\cliche-studio.exe"; DestDir: "{app}"; Flags: ignoreversion
; The Evergreen WebView2 bootstrapper — packed but extracted only if needed (below).
Source: "MicrosoftEdgeWebview2Setup.exe"; Flags: dontcopy

[Icons]
Name: "{userprograms}\Cliché Studio"; Filename: "{app}\cliche-studio.exe"
Name: "{userprograms}\Uninstall Cliché Studio"; Filename: "{uninstallexe}"
Name: "{autodesktop}\Cliché Studio"; Filename: "{app}\cliche-studio.exe"; Tasks: desktopicon

[Registry]
; Opt-in: append the install dir to the per-user PATH so `cliche` works in a terminal.
Root: HKCU; Subkey: "Environment"; ValueType: expandsz; ValueName: "Path"; ValueData: "{olddata};{app}"; Tasks: addtopath; Check: NeedsAddPath('{app}')

[Run]
Filename: "{app}\cliche-studio.exe"; Description: "Launch Cliché Studio now"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; Remove the regenerable config/state dir. The user's projects in
; ~/Cliche Projects are intentionally LEFT in place (their generated work).
Type: filesandordirs; Name: "{userappdata}\cliche"

[Code]
function WebView2Installed: Boolean;
var v: String;
begin
  Result :=
    (RegQueryStringValue(HKLM, 'SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}', 'pv', v) and (v <> '') and (v <> '0.0.0.0'))
    or (RegQueryStringValue(HKCU, 'SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}', 'pv', v) and (v <> '') and (v <> '0.0.0.0'));
end;

function NeedsAddPath(Param: String): Boolean;
var orig: String;
begin
  if not RegQueryStringValue(HKCU, 'Environment', 'Path', orig) then
  begin
    Result := True;
    exit;
  end;
  Result := Pos(';' + Uppercase(Param) + ';', ';' + Uppercase(orig) + ';') = 0;
end;

procedure CurStepChanged(CurStep: TSetupStep);
var code: Integer;
begin
  if (CurStep = ssInstall) and (not WebView2Installed) then
  begin
    ExtractTemporaryFile('MicrosoftEdgeWebview2Setup.exe');
    Exec(ExpandConstant('{tmp}\MicrosoftEdgeWebview2Setup.exe'), '/silent /install', '', SW_HIDE, ewWaitUntilTerminated, code);
  end;
end;
