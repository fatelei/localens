$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)
if (Get-Command uv -ErrorAction SilentlyContinue) {
    if (-not (Test-Path .venv\Scripts\python.exe)) {
        uv venv --python 3.12 .venv
        if ($LASTEXITCODE -ne 0) { throw '创建 Python 环境失败' }
    }
    uv pip install --python .venv\Scripts\python.exe -r engine/requirements.txt
} else {
    if (-not (Test-Path .venv\Scripts\python.exe)) {
        py -3.12 -m venv .venv
        if ($LASTEXITCODE -ne 0) { throw '请先安装 Python 3.12 或 uv' }
    }
    & .venv\Scripts\python.exe -m pip install -r engine/requirements.txt
}
if ($LASTEXITCODE -ne 0) { throw '安装模型依赖失败' }
Write-Host "开发推理环境已准备。源码应用会自动发现 .venv；完整发行包无需此步骤。"
