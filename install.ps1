# irm https://github.com/bjornaer/gleipnir/releases/latest/download/install.ps1 | iex
$ErrorActionPreference = 'Stop'

$OpenCodeVersion = '2.3.0'
$Repo = 'https://github.com/bjornaer/gleipnir/releases/latest/download'
$Bin = Join-Path $HOME '.opencode\bin' # same layout as on Unix; gleip looks here for opencode

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$tmp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid())
New-Item -ItemType Directory -Force -Path $tmp, $Bin | Out-Null

try {
    # gleip, verified against the release checksums
    $asset = "gleip-windows-$arch.exe"
    Invoke-WebRequest "$Repo/$asset" -OutFile "$tmp\$asset" -UseBasicParsing
    $sums = (Invoke-WebRequest "$Repo/checksums.txt" -UseBasicParsing).Content
    $want = ($sums -split "`n" | Where-Object { $_ -match " $([regex]::Escape($asset))$" }) -replace ' .*', ''
    if ((Get-FileHash "$tmp\$asset" -Algorithm SHA256).Hash -ne $want.Trim()) { throw "checksum mismatch for $asset" }
    # A running exe can't be overwritten on Windows but can be renamed (gleip --update runs from it)
    Remove-Item "$Bin\gleip.exe.old" -Force -ErrorAction SilentlyContinue
    if (Test-Path "$Bin\gleip.exe") { Move-Item "$Bin\gleip.exe" "$Bin\gleip.exe.old" -Force }
    Copy-Item "$tmp\$asset" "$Bin\gleip.exe" -Force

    # OpenCode has no PowerShell installer: fetch its npm package, verified against npm's sha512
    $ocArch = if ($arch -eq 'arm64') { 'arm64' } else { 'x64' }
    $meta = Invoke-RestMethod "https://registry.npmjs.org/@opencode%2fcli-windows-$ocArch/$OpenCodeVersion"
    Invoke-WebRequest $meta.dist.tarball -OutFile "$tmp\oc.tgz" -UseBasicParsing
    $sha = [Convert]::ToBase64String([Security.Cryptography.SHA512]::Create().ComputeHash([IO.File]::ReadAllBytes("$tmp\oc.tgz")))
    if ("sha512-$sha" -cne $meta.dist.integrity) { throw 'checksum mismatch for opencode' }
    tar -xzf "$tmp\oc.tgz" -C $tmp
    Copy-Item "$tmp\package\bin\opencode.exe" "$Bin\opencode.exe" -Force
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $Bin) {
    [Environment]::SetEnvironmentVariable('Path', "$userPath;$Bin", 'User')
}
& "$Bin\gleip.exe" --help
Write-Host "gleip installed to $Bin. Open a new terminal, then run: gleip setup"
Write-Host "Run gleip --help anytime to see this again."
