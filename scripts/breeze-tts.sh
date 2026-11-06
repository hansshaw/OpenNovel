#!/usr/bin/env bash
# Optional upstream runtime. No Python packages or model weights are bundled.
set -euo pipefail
umask 077

readonly BREEZE_SOURCE_URL='https://github.com/breezeblue-ai/breeze-tts.git'
readonly BREEZE_SOURCE_COMMIT='daa6cb2efe5471f09369229f322d20bb956cb3c3'
breeze_dir="${BREEZE_TTS_HOME:-${XDG_DATA_HOME:-${HOME}/.local/share}/ainovel/breeze-tts}"
breeze_python="${BREEZE_TTS_PYTHON:-python3}"
breeze_source="${breeze_dir}/source"
breeze_venv="${breeze_dir}/venv"
breeze_stamp="${breeze_dir}/.installed-commit"
breeze_stage=''
breeze_lock=''

usage() {
  cat <<'EOF'
Usage: bash scripts/breeze-tts.sh <install|start|check|help>

  install  Download pinned inference source and install Python dependencies.
           This downloads PyTorch packages, but does not download model weights.
  start    Run the installed API in the foreground. First use of a Hugging Face
           model ID downloads its weights. Ctrl+C stops the service.
  check    Check the installed source, dependencies and CUDA GPU; no downloads.
  doctor   Alias for check.

Environment:
  BREEZE_TTS_HOME      Runtime directory (default: $XDG_DATA_HOME/ainovel/breeze-tts
                       or $HOME/.local/share/ainovel/breeze-tts)
  BREEZE_TTS_PYTHON    Python >=3.10 executable used by install (default: python3)
  BREEZE_TTS_MODEL     Local checkpoint path or HF ID (default: BreezeBlue/breeze-tts-2)
  BREEZE_TTS_REVISION  Optional HF model revision; ignored for a local checkpoint
  BREEZE_TTS_HOST      Listening address (default: 127.0.0.1)
  BREEZE_TTS_PORT      Listening port (default: 7860)
  BREEZE_TTS_FAST      1 enables --fast-all; 0 uses eager inference (default: 0)
  HF_HOME             Optional standard Hugging Face cache directory

Requires Linux and an NVIDIA CUDA GPU; 12 GB VRAM is recommended for eager
inference, 24 GB for the fast path. The upstream API has no built-in authentication.
Source: Apache-2.0. Weights and self-hosted outputs: research/non-commercial only;
commercial use requires a separate written license from BreezeBlue.
See docs/breeze-tts.md for setup, remote access and license links.
EOF
}

die() { printf 'Breeze TTS: %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "Missing command: $1"; }

check_platform() {
  [[ "$(uname -s)" == Linux ]] || die 'Run the inference server on Linux; other devices can connect to it remotely.'
  [[ "$breeze_dir" == /* && "$breeze_dir" != / ]] || die 'BREEZE_TTS_HOME must be an absolute directory path other than /.'
  need git
}

check_source() {
  [[ -d "$breeze_source/.git" ]] || die "No managed source at $breeze_source. Run install first."
  local origin revision
  origin="$(git -C "$breeze_source" remote get-url origin)"
  [[ "$origin" == "$BREEZE_SOURCE_URL" || "$origin" == "${BREEZE_SOURCE_URL%.git}" ]] || die 'Existing source uses another Git origin; choose a different BREEZE_TTS_HOME.'
  revision="$(git -C "$breeze_source" rev-parse HEAD)"
  [[ "$revision" == "$BREEZE_SOURCE_COMMIT" ]] || die 'Existing source uses another revision; it will not be reset. Choose a different BREEZE_TTS_HOME.'
  [[ -z "$(git -C "$breeze_source" status --porcelain)" ]] || die 'Existing source has local changes; it will not be overwritten. Choose a different BREEZE_TTS_HOME.'
}

check_installed() {
  check_source
  [[ -x "$breeze_venv/bin/python" && -f "$breeze_stamp" ]] || die 'Python dependencies are not installed. Run install first.'
  [[ "$(cat "$breeze_stamp")" == "$BREEZE_SOURCE_COMMIT" ]] || die 'Installation stamp does not match the pinned source. Run install first.'
}

cleanup() {
  if [[ -n "$breeze_stage" ]]; then rm -rf -- "$breeze_stage"; fi
  if [[ -n "$breeze_lock" ]]; then rmdir -- "$breeze_lock"; fi
}

install_runtime() {
  check_platform
  need "$breeze_python"
  "$breeze_python" -c 'import sys; sys.exit(0 if sys.version_info >= (3, 10) else "Python >=3.10 is required")'
  # Resolve symlinks before creating anything inside the chosen directory.
  breeze_dir="$("$breeze_python" -c 'import os, sys; print(os.path.realpath(sys.argv[1]))' "$breeze_dir")"
  [[ "$breeze_dir" != / ]] || die 'BREEZE_TTS_HOME must not resolve to /.'
  local project_root
  project_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
  case "$breeze_dir/" in
    "$project_root/"*) die 'Choose BREEZE_TTS_HOME outside the application checkout.' ;;
  esac
  mkdir -p -- "$breeze_dir"
  breeze_source="$breeze_dir/source"
  breeze_venv="$breeze_dir/venv"
  breeze_stamp="$breeze_dir/.installed-commit"
  mkdir -- "$breeze_dir/.install-lock" 2>/dev/null || die "An install is already running, or an interrupted install left $breeze_dir/.install-lock."
  breeze_lock="$breeze_dir/.install-lock"
  trap cleanup EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM

  if [[ ! -e "$breeze_source" && ! -L "$breeze_source" ]]; then
    breeze_stage="$(mktemp -d "$breeze_dir/.source-XXXXXX")"
    git init --quiet "$breeze_stage"
    git -C "$breeze_stage" remote add origin "$BREEZE_SOURCE_URL"
    git -C "$breeze_stage" fetch --depth 1 origin "$BREEZE_SOURCE_COMMIT"
    git -C "$breeze_stage" checkout --quiet --detach FETCH_HEAD
    mv -- "$breeze_stage" "$breeze_source"
    breeze_stage=''
  fi
  check_source
  if [[ -f "$breeze_stamp" && "$(cat "$breeze_stamp")" == "$BREEZE_SOURCE_COMMIT" && -x "$breeze_venv/bin/python" ]]; then
    "$breeze_venv/bin/python" -m pip check
    printf 'Breeze TTS is already installed at %s\n' "$breeze_dir"
    return
  fi
  if [[ ! -e "$breeze_venv" && ! -L "$breeze_venv" ]]; then
    "$breeze_python" -m venv "$breeze_venv"
  fi
  [[ -f "$breeze_venv/pyvenv.cfg" && -x "$breeze_venv/bin/python" ]] || die 'Existing venv directory is not a Python virtual environment; choose a different BREEZE_TTS_HOME.'
  # A failed venv creation (for example before python3-venv was installed) can
  # leave a valid interpreter and config without pip. Repair only that missing
  # bootstrap step so retrying install preserves the existing environment.
  if ! "$breeze_venv/bin/python" -m pip --version >/dev/null 2>&1; then
    "$breeze_venv/bin/python" -m ensurepip --upgrade || die 'Cannot bootstrap pip. Install the venv/ensurepip system package for this Python version, then retry install.'
  fi
  "$breeze_venv/bin/python" -m pip install --require-virtualenv -r "$breeze_source/requirements.txt"
  "$breeze_venv/bin/python" -m pip check
  printf '%s\n' "$BREEZE_SOURCE_COMMIT" > "$breeze_stamp"
  printf 'Installed source and dependencies at %s. Model weights have not been downloaded.\n' "$breeze_dir"
  printf 'Next: bash scripts/breeze-tts.sh check, then bash scripts/breeze-tts.sh start\n'
}

check_cuda() {
  "$breeze_venv/bin/python" -c '
import sys
import torch
if not torch.cuda.is_available():
    sys.exit("CUDA is unavailable to PyTorch. Check the NVIDIA driver and GPU visibility.")
device = torch.cuda.get_device_properties(torch.cuda.current_device())
print(f"CUDA: {device.name}; VRAM: {device.total_memory / 2**30:.1f} GiB; PyTorch: {torch.__version__}")
'
}

check_runtime() {
  check_platform
  check_installed
  "$breeze_venv/bin/python" -m pip check
  check_cuda
  (cd -- "$breeze_source" && "$breeze_venv/bin/python" -c 'import breeze_infer.api; print("Breeze API imports successfully.")')
  printf 'Source: %s\nRuntime: %s\n' "$BREEZE_SOURCE_COMMIT" "$breeze_dir"
  printf 'Checks passed; model weights have not been loaded and no audio has been generated.\n'
}

start_runtime() {
  check_platform
  check_installed
  local model host port fast
  model="${BREEZE_TTS_MODEL:-BreezeBlue/breeze-tts-2}"
  host="${BREEZE_TTS_HOST:-127.0.0.1}"
  port="${BREEZE_TTS_PORT:-7860}"
  fast="${BREEZE_TTS_FAST:-0}"
  [[ "$port" =~ ^[0-9]{1,5}$ && "$((10#$port))" -ge 1 && "$((10#$port))" -le 65535 ]] || die 'BREEZE_TTS_PORT must be between 1 and 65535.'
  [[ "$fast" == 0 || "$fast" == 1 ]] || die 'BREEZE_TTS_FAST must be 0 or 1.'
  [[ -n "$host" && "$host" != -* && "$model" != -* ]] || die 'Invalid host or model.'
  # Keep relative local checkpoints relative to the caller, not the source tree.
  if [[ -d "$model" ]]; then model="$(cd -- "$model" && pwd -P)"; fi
  check_cuda
  local -a args=(-m breeze_infer.api "$model" --host "$host" --port "$port")
  if [[ -n "${BREEZE_TTS_REVISION:-}" ]]; then args+=(--revision "$BREEZE_TTS_REVISION"); fi
  if [[ "$fast" == 1 ]]; then args+=(--fast-all); fi
  printf 'Starting Breeze TTS at %s:%s; Ctrl+C stops the service.\n' "$host" "$port"
  printf 'Model: %s (a Hugging Face ID may download weights on first start).\n' "$model"
  cd -- "$breeze_source"
  exec "$breeze_venv/bin/python" "${args[@]}"
}

[[ $# -le 1 ]] || { usage >&2; exit 2; }
case "${1:-help}" in
  install) install_runtime ;;
  start) start_runtime ;;
  check|doctor) check_runtime ;;
  help|-h|--help) usage ;;
  *) usage >&2; exit 2 ;;
esac
