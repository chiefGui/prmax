@echo off
setlocal
set VSWHERE=%ProgramFiles(x86)%\Microsoft Visual Studio\Installer\vswhere.exe
for /f "usebackq delims=" %%i in (`"%VSWHERE%" -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath`) do set VS=%%i
if not defined VS (
    echo Visual Studio C++ tools not found.
    exit /b 1
)
call "%VS%\VC\Auxiliary\Build\vcvars64.bat" >nul || exit /b 1
cd /d "%~dp0"
cmake -S . -B build -G Ninja -DCMAKE_BUILD_TYPE=Release || exit /b 1
cmake --build build || exit /b 1
del ..\bin\prmax-desktop.old 2>nul
if exist ..\bin\prmax-desktop.exe move /y ..\bin\prmax-desktop.exe ..\bin\prmax-desktop.old >nul
copy /y build\prmax-desktop.exe ..\bin\prmax-desktop.exe >nul || exit /b 1
del ..\bin\prmax-desktop.old 2>nul
