@echo off
setlocal
cd /d "%~dp0\..\.."
if not exist native\bin mkdir native\bin
if not defined VCVARS64 set "VCVARS64=C:\Program Files (x86)\Microsoft Visual Studio\2017\BuildTools\VC\Auxiliary\Build\vcvars64.bat"
if not exist "%VCVARS64%" (
  echo Set VCVARS64 to the vcvars64.bat supplied with your Visual Studio C++ tools.
  exit /b 1
)
call "%VCVARS64%"
if errorlevel 1 exit /b 1
cl /nologo /std:c++17 /EHsc /O2 /MT /LD /DUNICODE /D_UNICODE /Fonative\bin\ /Ithird_party\vst3 native\vst3\bridge.cpp third_party\vst3\pluginterfaces\base\funknown.cpp /link /OUT:native\bin\QuietLabVST3.dll /IMPLIB:native\bin\QuietLabVST3.lib ole32.lib user32.lib
if errorlevel 1 exit /b 1
if not exist native\test mkdir native\test
cl /nologo /std:c++17 /EHsc /O2 /MT /LD /DUNICODE /D_UNICODE /Fonative\test\ /Ithird_party\vst3 native\vst3\test_gain.cpp third_party\vst3\pluginterfaces\base\funknown.cpp /link /OUT:native\test\QuietLabTestGain.vst3 /IMPLIB:native\test\QuietLabTestGain.lib ole32.lib user32.lib
if errorlevel 1 exit /b 1
go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -ico assets/quiet-lab.ico -o resource_windows_amd64.syso
exit /b %errorlevel%
