@echo off
echo Installing netpulse to %USERPROFILE%\.netpulse\bin...
mkdir "%USERPROFILE%\.netpulse\bin" 2>nul
copy /Y "%~dp0netpulse.exe" "%USERPROFILE%\.netpulse\bin\netpulse.exe"
setx PATH "%USERPROFILE%\.netpulse\bin;%PATH%"
echo.
echo Installed! Open a NEW terminal and run: netpulse
pause