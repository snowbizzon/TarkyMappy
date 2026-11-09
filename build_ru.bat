@echo off
setlocal
chcp 65001 >nul
title TarkyMappy build assistant [RU]
set "AUTO="
if /I "%~1"=="/auto" set "AUTO=-Auto"
if /I "%~1"=="--auto" set "AUTO=-Auto"
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0tools\build.ps1" -Lang ru %AUTO%
set "RC=%ERRORLEVEL%"
if not defined AUTO pause
exit /b %RC%
