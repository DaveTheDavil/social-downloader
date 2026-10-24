@echo off
title Social Downloader - instalcja
rem Kreator instalacji (GUI). Po instalacji skrot "Social Downloader"
rem uruchamia silnik (cicho) i klienta razem.
if not exist "%~dp0app\SocialDownloader.exe" (
  echo BLAD: uruchom INSTALATOR.bat z ROZPAKOWANEGO folderu paczki.
  pause
  exit /b 1
)
powershell -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "%~dp0installer\instalator.ps1"
if errorlevel 1 pause
