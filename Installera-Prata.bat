@echo off
chcp 65001 >nul
title Installera Prata
echo.
echo   Installerar Prata (maskinbred, autostart för alla användare).
echo   Godkänn UAC-rutan som dyker upp.
echo.
"%~dp0prata.exe" --install || pause
