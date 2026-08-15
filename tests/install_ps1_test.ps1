$ErrorActionPreference = "Stop"
Set-StrictMode -Version 3.0

function Assert-True([bool] $Condition, [string] $Message) {
    if (-not $Condition) { throw "Assertion failed: $Message" }
}

$repositoryRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $repositoryRoot "install.ps1")

$testRoot = Join-Path ([IO.Path]::GetTempPath()) ("luban-code-installer-test-" + [Guid]::NewGuid().ToString("N"))
$fixtureRoot = Join-Path $testRoot "fixtures"
$installRoot = Join-Path $testRoot "install"
New-Item -ItemType Directory -Path $fixtureRoot | Out-Null

try {
    function Get-LubanWindowsArchitecture { return "x86_64" }
    function Get-LubanReleaseVersion { param($RequestedVersion, $Repo); return $RequestedVersion }
    function Add-LubanUserPath { param($Directory) }
    function Remove-LubanUserPath { param($Directory) }

    $payloadRoot = Join-Path $testRoot "payload"
    New-Item -ItemType Directory -Path $payloadRoot | Out-Null
    Set-Content -LiteralPath (Join-Path $payloadRoot "luban-code.exe") -Value "version-one" -NoNewline
    $assetName = "luban-code_Windows_x86_64.zip"
    $archiveFixture = Join-Path $fixtureRoot $assetName
    Compress-Archive -LiteralPath (Join-Path $payloadRoot "luban-code.exe") -DestinationPath $archiveFixture
    $hash = (Get-FileHash -LiteralPath $archiveFixture -Algorithm SHA256).Hash.ToLowerInvariant()
    Set-Content -LiteralPath (Join-Path $fixtureRoot "checksums.txt") -Value "$hash  $assetName"

    function Invoke-LubanDownload {
        param($Uri, $Destination)
        Copy-Item -LiteralPath (Join-Path $fixtureRoot (Split-Path -Leaf $Uri)) -Destination $Destination
    }

    Install-LubanCode -RequestedVersion "v1.2.3" -Destination $installRoot -Repo "test/repo" -BaseUrl "https://example.invalid" -SkipPath
    Assert-True (Test-Path -LiteralPath (Join-Path $installRoot "luban-code.exe")) "executable is installed"
    Assert-True ((Get-Content -Raw -LiteralPath (Join-Path $installRoot "luban-code.exe")) -eq "version-one") "installed executable matches archive"
    Assert-True ((Get-Content -Raw -LiteralPath (Join-Path $installRoot ".luban-code-install")).Trim() -eq "v1.2.3") "marker records version"

    Set-Content -LiteralPath (Join-Path $payloadRoot "luban-code.exe") -Value "version-two" -NoNewline
    Remove-Item -LiteralPath $archiveFixture
    Compress-Archive -LiteralPath (Join-Path $payloadRoot "luban-code.exe") -DestinationPath $archiveFixture
    $hash = (Get-FileHash -LiteralPath $archiveFixture -Algorithm SHA256).Hash.ToLowerInvariant()
    Set-Content -LiteralPath (Join-Path $fixtureRoot "checksums.txt") -Value "$hash *$assetName"
    Install-LubanCode -RequestedVersion "v1.2.4" -Destination $installRoot -Repo "test/repo" -BaseUrl "https://example.invalid" -SkipPath
    Assert-True ((Get-Content -Raw -LiteralPath (Join-Path $installRoot "luban-code.exe")) -eq "version-two") "upgrade replaces executable"

    Set-Content -LiteralPath (Join-Path $fixtureRoot "checksums.txt") -Value "$('0' * 64)  $assetName"
    $failed = $false
    try {
        Install-LubanCode -RequestedVersion "v1.2.5" -Destination $installRoot -Repo "test/repo" -BaseUrl "https://example.invalid" -SkipPath
    }
    catch { $failed = $_.Exception.Message -like "Checksum verification failed*" }
    Assert-True $failed "bad checksum is rejected"
    Assert-True ((Get-Content -Raw -LiteralPath (Join-Path $installRoot "luban-code.exe")) -eq "version-two") "bad checksum preserves existing installation"

    Set-Content -LiteralPath (Join-Path $installRoot "keep.txt") -Value "user-file"
    Uninstall-LubanCode -Destination $installRoot
    Assert-True (-not (Test-Path -LiteralPath (Join-Path $installRoot "luban-code.exe"))) "uninstall removes executable"
    Assert-True (Test-Path -LiteralPath (Join-Path $installRoot "keep.txt")) "uninstall preserves unrelated files"

    $invalidVersionFailed = $false
    try { Assert-LubanVersionTag -RequestedVersion "1.2" }
    catch { $invalidVersionFailed = $_.Exception.Message -like "Invalid version*" }
    Assert-True $invalidVersionFailed "invalid versions are rejected"

    $unmanagedRoot = Join-Path $testRoot "unmanaged"
    New-Item -ItemType Directory -Path $unmanagedRoot | Out-Null
    Set-Content -LiteralPath (Join-Path $unmanagedRoot "luban-code.exe") -Value "unmanaged"
    $unmanagedFailed = $false
    try { Uninstall-LubanCode -Destination $unmanagedRoot }
    catch { $unmanagedFailed = $_.Exception.Message -like "Refusing to remove*" }
    Assert-True $unmanagedFailed "uninstall refuses an unmanaged executable"
    Assert-True (Test-Path -LiteralPath (Join-Path $unmanagedRoot "luban-code.exe")) "unmanaged executable is preserved"

    Write-Host "install.ps1 tests passed"
}
finally {
    if (Test-Path -LiteralPath $testRoot) { Remove-Item -LiteralPath $testRoot -Recurse -Force }
}
