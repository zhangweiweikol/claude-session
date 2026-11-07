@echo off
taskkill /F /IM session-deck.exe >nul 2>&1
echo Session Deck stopped.
timeout /t 2 >nul
