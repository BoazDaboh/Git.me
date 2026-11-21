#ifndef SourceDir
  #error Define SourceDir as the path to a freshly built Git/Me package.
#endif

[Setup]
AppId={{F5F3E51D-3579-47EB-85CF-0D3C089B28DC}
AppName=GitMe
AppVersion=0.1.0
AppPublisher=GitMe
DefaultDirName={localappdata}\Programs\GitMe
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=..\dist\installer
OutputBaseFilename=GitMe-Setup
Compression=lzma2/fast
SolidCompression=yes
WizardStyle=modern
UninstallDisplayName=GitMe

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Additional shortcuts:"

[Files]
Source: "{#SourceDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\GitMe"; Filename: "{app}\Start GitMe.cmd"; WorkingDir: "{app}"
Name: "{autodesktop}\GitMe"; Filename: "{app}\Start GitMe.cmd"; WorkingDir: "{app}"; Tasks: desktopicon
