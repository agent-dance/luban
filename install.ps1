[CmdletBinding()]
param(
    [string] $Version = "latest",
    [string] $InstallDir = (Join-Path ([Environment]::GetFolderPath("LocalApplicationData")) "Programs\luban-code\bin"),
    [switch] $Uninstall,
    [switch] $NoPath,
    [switch] $Help,
    [string] $Repository = "agent-dance/luban",
    [string] $DownloadBaseUrl = "https://github.com"
)

Set-StrictMode -Version 3.0
$ErrorActionPreference = "Stop"

$script:ProductName = "LUBAN Code"
$script:ExecutableName = "luban.exe"
$script:InstallMarkerName = ".luban-code-install"

function Show-LubanInstallerHelp {
    @"
Install or update LUBAN Code from an official GitHub Release.

Usage:
  .\install.ps1 [-Version <vX.Y.Z>] [-InstallDir <directory>] [-NoPath]
  .\install.ps1 -Uninstall [-InstallDir <directory>]
  .\install.ps1 -Help

Options:
  -Version          Release tag to install. Defaults to latest.
  -InstallDir       Destination directory for luban.exe.
  -NoPath           Do not add the destination to the user PATH.
  -Uninstall        Remove the script-managed executable and PATH entry.
  -Repository       GitHub owner/repository. Intended for mirrors and testing.
  -DownloadBaseUrl  GitHub-compatible base URL. Intended for mirrors and testing.

The installer verifies the release archive against checksums.txt before replacing
an existing installation. It never requests administrator privileges.
"@
}

function Get-LubanWindowsArchitecture {
    $isWindowsHost = $env:OS -eq "Windows_NT"
    $isWindowsVariable = Get-Variable -Name IsWindows -ErrorAction SilentlyContinue
    if ($null -ne $isWindowsVariable) { $isWindowsHost = [bool] $isWindowsVariable.Value }
    if (-not $isWindowsHost) {
        throw "This installer only supports Windows. Download the archive manually on another operating system."
    }

    $architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
    switch ($architecture) {
        "X64"   { return "x86_64" }
        default { throw "Unsupported Windows architecture: $architecture. This release supports Windows x86_64." }
    }
}

function Assert-LubanVersionTag {
    param([Parameter(Mandatory)] [string] $RequestedVersion)
    if ($RequestedVersion -ne "latest" -and $RequestedVersion -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$') {
        throw "Invalid version '$RequestedVersion'. Use a release tag such as v0.1.0 or 'latest'."
    }
}

function Get-LubanReleaseVersion {
    param(
        [Parameter(Mandatory)] [string] $RequestedVersion,
        [Parameter(Mandatory)] [string] $Repo
    )

    Assert-LubanVersionTag -RequestedVersion $RequestedVersion
    if ($RequestedVersion -ne "latest") { return $RequestedVersion }

    $headers = @{ Accept = "application/vnd.github+json"; "X-GitHub-Api-Version" = "2022-11-28" }
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $headers
    }
    catch {
        throw "Unable to determine the latest release for $Repo. Check the network connection or pass -Version explicitly. $($_.Exception.Message)"
    }
    if ([string]::IsNullOrWhiteSpace([string] $release.tag_name)) {
        throw "The latest release response for $Repo did not contain a tag name."
    }
    $resolvedVersion = [string] $release.tag_name
    Assert-LubanVersionTag -RequestedVersion $resolvedVersion
    return $resolvedVersion
}

function Invoke-LubanDownload {
    param(
        [Parameter(Mandatory)] [string] $Uri,
        [Parameter(Mandatory)] [string] $Destination
    )

    try {
        Invoke-WebRequest -Uri $Uri -OutFile $Destination -UseBasicParsing
    }
    catch {
        throw "Download failed: $Uri. $($_.Exception.Message)"
    }
}

function Assert-LubanChecksum {
    param(
        [Parameter(Mandatory)] [string] $ArchivePath,
        [Parameter(Mandatory)] [string] $ChecksumsPath,
        [Parameter(Mandatory)] [string] $AssetName
    )

    $escapedName = [Regex]::Escape($AssetName)
    $matches = @(Get-Content -LiteralPath $ChecksumsPath | Where-Object {
        $_ -match "^([A-Fa-f0-9]{64})\s+\*?$escapedName$"
    })
    if ($matches.Count -ne 1) {
        throw "checksums.txt must contain exactly one SHA-256 entry for $AssetName."
    }
    $null = $matches[0] -match '^([A-Fa-f0-9]{64})'
    $expected = $Matches[1].ToLowerInvariant()
    $actual = (Get-FileHash -LiteralPath $ArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) {
        throw "Checksum verification failed for $AssetName. Expected $expected but downloaded $actual. The existing installation was not changed."
    }
}

function Get-LubanUserPathEntries {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ([string]::IsNullOrWhiteSpace($userPath)) { return @() }
    return @($userPath.Split(';', [StringSplitOptions]::RemoveEmptyEntries))
}

function Set-LubanUserPathEntries {
    param([Parameter(Mandatory)] [string[]] $Entries)
    [Environment]::SetEnvironmentVariable("Path", ($Entries -join ';'), "User")
}

function Add-LubanUserPath {
    param([Parameter(Mandatory)] [string] $Directory)

    $fullDirectory = [IO.Path]::GetFullPath($Directory).TrimEnd('\')
    $entries = @(Get-LubanUserPathEntries)
    if (-not ($entries | Where-Object { [IO.Path]::GetFullPath($_).TrimEnd('\') -ieq $fullDirectory })) {
        Set-LubanUserPathEntries -Entries @($entries + $fullDirectory)
        Write-Host "Added $fullDirectory to the user PATH. Open a new terminal to use luban."
    }
    if (-not (($env:Path -split ';') -contains $fullDirectory)) {
        $env:Path = "$fullDirectory;$env:Path"
    }
}

function Remove-LubanUserPath {
    param([Parameter(Mandatory)] [string] $Directory)

    $fullDirectory = [IO.Path]::GetFullPath($Directory).TrimEnd('\')
    $entries = @(Get-LubanUserPathEntries | Where-Object {
        [IO.Path]::GetFullPath($_).TrimEnd('\') -ine $fullDirectory
    })
    Set-LubanUserPathEntries -Entries $entries
}

function Uninstall-LubanCode {
    param([Parameter(Mandatory)] [string] $Destination)

    $fullDestination = [IO.Path]::GetFullPath($Destination)
    $executable = Join-Path $fullDestination $script:ExecutableName
    $legacyExecutable = Join-Path $fullDestination "luban-code.exe"
    $marker = Join-Path $fullDestination $script:InstallMarkerName

    if (Test-Path -LiteralPath $marker -PathType Leaf) {
        Remove-LubanUserPath -Directory $fullDestination
        Remove-Item -LiteralPath $executable -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $legacyExecutable -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $marker -Force
        $remaining = @(Get-ChildItem -LiteralPath $fullDestination -Force -ErrorAction SilentlyContinue)
        if ($remaining.Count -eq 0) { Remove-Item -LiteralPath $fullDestination -Force }
        Write-Host "$script:ProductName was uninstalled from $fullDestination. User configuration and session data were preserved."
        return
    }
    if (Test-Path -LiteralPath $executable -PathType Leaf) {
        throw "Refusing to remove $executable because it was not installed by this script. Remove it manually if appropriate."
    }
    Remove-LubanUserPath -Directory $fullDestination
    Write-Host "$script:ProductName is not installed in $fullDestination. Any matching user PATH entry was removed."
}

function Install-LubanCode {
    param(
        [Parameter(Mandatory)] [string] $RequestedVersion,
        [Parameter(Mandatory)] [string] $Destination,
        [Parameter(Mandatory)] [string] $Repo,
        [Parameter(Mandatory)] [string] $BaseUrl,
        [switch] $SkipPath
    )

    $architecture = Get-LubanWindowsArchitecture
    $releaseVersion = Get-LubanReleaseVersion -RequestedVersion $RequestedVersion -Repo $Repo
    $assetName = "luban-code_Windows_$architecture.zip"
    $releaseRoot = "$($BaseUrl.TrimEnd('/'))/$Repo/releases/download/$releaseVersion"
    $temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ("luban-code-install-" + [Guid]::NewGuid().ToString("N"))
    $stagedExecutable = $null

    New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
    try {
        $archivePath = Join-Path $temporaryDirectory $assetName
        $checksumsPath = Join-Path $temporaryDirectory "checksums.txt"
        Invoke-LubanDownload -Uri "$releaseRoot/$assetName" -Destination $archivePath
        Invoke-LubanDownload -Uri "$releaseRoot/checksums.txt" -Destination $checksumsPath
        Assert-LubanChecksum -ArchivePath $archivePath -ChecksumsPath $checksumsPath -AssetName $assetName

        $extractPath = Join-Path $temporaryDirectory "archive"
        Expand-Archive -LiteralPath $archivePath -DestinationPath $extractPath
        $candidates = @(Get-ChildItem -LiteralPath $extractPath -Filter $script:ExecutableName -File -Recurse)
        if ($candidates.Count -ne 1) {
            throw "The release archive must contain exactly one $script:ExecutableName file; found $($candidates.Count)."
        }

        $fullDestination = [IO.Path]::GetFullPath($Destination)
        New-Item -ItemType Directory -Path $fullDestination -Force | Out-Null
        $destinationExecutable = Join-Path $fullDestination $script:ExecutableName
        $legacyExecutable = Join-Path $fullDestination "luban-code.exe"
        $stagedExecutable = Join-Path $fullDestination (".$script:ExecutableName.new")
        Copy-Item -LiteralPath $candidates[0].FullName -Destination $stagedExecutable -Force
        Move-Item -LiteralPath $stagedExecutable -Destination $destinationExecutable -Force
        if (Test-Path -LiteralPath (Join-Path $fullDestination $script:InstallMarkerName) -PathType Leaf) {
            Remove-Item -LiteralPath $legacyExecutable -Force -ErrorAction SilentlyContinue
        }
        Set-Content -LiteralPath (Join-Path $fullDestination $script:InstallMarkerName) -Value $releaseVersion -Encoding ASCII

        if (-not $SkipPath) { Add-LubanUserPath -Directory $fullDestination }
        Write-Host "$script:ProductName $releaseVersion was installed to $destinationExecutable."
        if ($SkipPath) { Write-Host "PATH was not changed. Run $destinationExecutable directly or add its directory to PATH." }
    }
    finally {
        if ($null -ne $stagedExecutable -and (Test-Path -LiteralPath $stagedExecutable)) {
            Remove-Item -LiteralPath $stagedExecutable -Force
        }
        if (Test-Path -LiteralPath $temporaryDirectory) {
            Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force
        }
    }
}

function Invoke-LubanInstaller {
    if ($Help) { Show-LubanInstallerHelp; return }
    if ($Uninstall) { Uninstall-LubanCode -Destination $InstallDir; return }
    Install-LubanCode -RequestedVersion $Version -Destination $InstallDir -Repo $Repository -BaseUrl $DownloadBaseUrl -SkipPath:$NoPath
}

if ($MyInvocation.InvocationName -ne '.') {
    try {
        Invoke-LubanInstaller
    }
    catch {
        Write-Error $_.Exception.Message
        exit 1
    }
}
