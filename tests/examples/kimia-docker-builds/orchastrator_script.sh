#!/bin/bash
set -u
SCRIPTS_DIR="./examples"

RUN_STAMP="$(date +%Y%m%d-%H%M%S)"
LOG_DIR="./logs/$RUN_STAMP"
mkdir -p "$LOG_DIR"

declare -A RESULTS=()
any=false
overall_status=0

for script in "$SCRIPTS_DIR"/*.sh; do
    [[ -e "$script" ]] || continue
    any=true

    base="$(basename "$script" .sh)"
    logfile="$LOG_DIR/${base}.log"

    echo "==> Running: $script"
    echo "    Log: $logfile"
    echo "------------------------------------------------------------"

    {
        echo "=== SCRIPT: $script"
        echo "=== START : $(date -Is)"
    } >> "$logfile"

    SECONDS=0
    if [[ -x "$script" ]]; then
        (
        "$script"
        ) > >(tee -a "$logfile") 2> >(tee -a "$logfile" >&2)
        rc=$?
    else
        echo "    (not executable; running with bash)"
        (
        bash "$script"
        ) > >(tee -a "$logfile") 2> >(tee -a "$logfile" >&2)
        rc=$?
    fi

    duration="${SECONDS}s"
    {
        echo "=== END   : $(date -Is)"
        echo "=== EXIT  : $rc"
        echo "=== TIME  : $duration"
        echo
    } >> "$logfile"

    if (( rc == 0 )); then
        echo "✅  Done: $script  (exit $rc, ${duration})"
        RESULTS["$base"]="OK ($duration)"
    else
        echo "❌  FAILED: $script  (exit $rc, ${duration})"
        RESULTS["$base"]="FAIL (exit $rc, ${duration})"
        overall_status=1
    fi

  echo "------------------------------------------------------------"
  echo
done

if ! $any; then
  echo "No .sh files found in: $SCRIPTS_DIR"
  exit 0
fi

# Summary
summary_file="$LOG_DIR/_summary.txt"
{
  echo "Run started:  $(date -Is -r "$SECONDS" 2>/dev/null || date -Is)"
  echo "Logs folder:  $LOG_DIR"
  echo
  echo "Summary:"
  for name in "${!RESULTS[@]}"; do
    printf " - %-30s : %s\n" "$name" "${RESULTS[$name]}"
  done
} | tee "$summary_file"

echo
echo "Summary saved to: $summary_file"
exit $overall_status