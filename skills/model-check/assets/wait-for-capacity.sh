#!/bin/sh
# wait-for-capacity.sh — block until the machine has room for a model-checking run.
#
# Run it before `mcd check`, before starting `mcd serve`, and before any batch of
# runs (a test campaign, an oracle, a benchmark). A run uses CPU and memory in
# proportion to the state space, and the machine may be shared with other
# sessions: starting into a saturated machine slows every run down, can push the
# machine into swap, and makes a time budget stop a search early (`unknown`).
#
# Usage:
#   wait-for-capacity.sh [--max-cpu PCT] [--max-mem PCT] [--need-cores N]
#                        [--need-mem-mb MB] [--dir PATH --min-disk-mb MB]
#                        [--timeout SEC] [--interval SEC] [--quiet]
#
#   --max-cpu PCT      busy CPU above this percentage is "busy" (default 90)
#   --max-mem PCT      used memory above this percentage is "busy" (default 90)
#   --need-cores N     cores the run will use (e.g. --workers N): the run fits
#                      when busy CPU plus N/ncpu stays within --max-cpu
#   --need-mem-mb MB   memory the run will take (its --budget-mem-mb): the run fits
#                      when used memory plus MB stays within --max-mem
#   --dir PATH         a directory the run writes to (the session directory,
#                      TMPDIR); with --min-disk-mb the free space there is checked
#   --timeout SEC      give up after SEC seconds of waiting (default 900);
#                      0 = look once and do not wait
#   --interval SEC     seconds between looks (default 5)
#   --quiet            no progress lines on standard error
#
# Exit codes: 0 there is room; 3 still busy when the timeout ended (do not start
# the run: queue it, lower --workers or the budgets, or ask the user); 2 usage
# error; 4 the machine cannot be measured here (check it by hand, for example
# with uptime, free and df, before you start).
#
# CPU: Linux reads /proc/stat over one second; macOS uses the one-minute load
# average divided by the number of CPUs. Memory: MemAvailable on Linux; free,
# inactive and speculative pages on macOS. Windows is not covered (exit 4).

max_cpu=90 max_mem=90 need_cores=0 need_mem=0 dir="" min_disk=0 timeout=900 interval=5 quiet=0

usage() { sed -n '2,32p' "$0" >&2; exit 2; }
num() { case "$1" in ''|*[!0-9]*) echo "wait-for-capacity: not a number: $1" >&2; exit 2 ;; esac; }

while [ $# -gt 0 ]; do
  case "$1" in
    --max-cpu)     [ $# -ge 2 ] || usage; num "$2"; max_cpu=$2; shift 2 ;;
    --max-mem)     [ $# -ge 2 ] || usage; num "$2"; max_mem=$2; shift 2 ;;
    --need-cores)  [ $# -ge 2 ] || usage; num "$2"; need_cores=$2; shift 2 ;;
    --need-mem-mb) [ $# -ge 2 ] || usage; num "$2"; need_mem=$2; shift 2 ;;
    --dir)         [ $# -ge 2 ] || usage; dir=$2; shift 2 ;;
    --min-disk-mb) [ $# -ge 2 ] || usage; num "$2"; min_disk=$2; shift 2 ;;
    --timeout)     [ $# -ge 2 ] || usage; num "$2"; timeout=$2; shift 2 ;;
    --interval)    [ $# -ge 2 ] || usage; num "$2"; interval=$2; shift 2 ;;
    --quiet)       quiet=1; shift ;;
    -h|--help)     usage ;;
    *) echo "wait-for-capacity: unknown option $1" >&2; usage ;;
  esac
done
[ "$interval" -ge 1 ] 2>/dev/null || interval=1

os=$(uname -s 2>/dev/null)

# measure prints: cpu_busy_pct mem_used_pct mem_total_mb ncpu   (integers)
measure() {
  case "$os" in
    Linux)
      [ -r /proc/stat ] && [ -r /proc/meminfo ] || return 1
      a=$(head -1 /proc/stat); sleep 1; b=$(head -1 /proc/stat)
      ncpu=$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 1)
      cpu=$(printf '%s\n%s\n' "$a" "$b" | awk '
        { idle=$5+$6; tot=0; for (i=2;i<=NF;i++) tot+=$i
          if (NR==1) { i0=idle; t0=tot } else { di=idle-i0; dt=tot-t0 } }
        END { if (dt>0) printf "%d", 100*(1-di/dt); else print 0 }')
      mem=$(awk '/^MemTotal:/{t=$2} /^MemAvailable:/{a=$2}
        END { if (t>0) printf "%d %d", 100*(1-a/t), t/1024; else print "0 0" }' /proc/meminfo)
      echo "$cpu $mem $ncpu" ;;
    Darwin)
      ncpu=$(sysctl -n hw.ncpu 2>/dev/null) || return 1
      load=$(sysctl -n vm.loadavg 2>/dev/null | awk '{print $2}')
      cpu=$(awk -v l="$load" -v n="$ncpu" 'BEGIN { p=100*l/n; if (p>100) p=100; printf "%d", p }')
      total=$(sysctl -n hw.memsize 2>/dev/null) || return 1
      mem=$(vm_stat 2>/dev/null | awk -v tot="$total" '
        /page size of/ { ps=$8 }
        /Pages free/ { f=$3 } /Pages inactive/ { i=$3 } /Pages speculative/ { s=$3 }
        END { gsub(/\./,"",f); gsub(/\./,"",i); gsub(/\./,"",s)
              av=(f+i+s)*ps; if (tot>0) printf "%d %d", 100*(1-av/tot), tot/1048576; else print "0 0" }')
      echo "$cpu $mem $ncpu" ;;
    *) return 1 ;;
  esac
}

waited=0
while :; do
  m=$(measure) || { echo "wait-for-capacity: cannot measure this machine ($os); check CPU, memory and disk by hand" >&2; exit 4; }
  set -- $m; cpu=$1 mem=$2 total=$3 ncpu=$4

  # what the run would add
  add_cpu=0; [ "$need_cores" -gt 0 ] && add_cpu=$(awk -v c="$need_cores" -v n="$ncpu" 'BEGIN { printf "%d", 100*c/n }')
  add_mem=0; [ "$need_mem" -gt 0 ] && [ "$total" -gt 0 ] && add_mem=$(awk -v m="$need_mem" -v t="$total" 'BEGIN { printf "%d", 100*m/t }')

  why=""
  [ $((cpu + add_cpu)) -gt "$max_cpu" ] && why="$why cpu ${cpu}%+${add_cpu}% over ${max_cpu}%;"
  [ $((mem + add_mem)) -gt "$max_mem" ] && why="$why mem ${mem}%+${add_mem}% over ${max_mem}%;"
  if [ -n "$dir" ] && [ "$min_disk" -gt 0 ]; then
    free_mb=$(df -Pk "$dir" 2>/dev/null | awk 'NR==2 { printf "%d", $4/1024 }')
    if [ -z "$free_mb" ]; then
      echo "wait-for-capacity: cannot read the free space of $dir" >&2; exit 4
    fi
    [ "$free_mb" -lt "$min_disk" ] && why="$why disk ${free_mb}MB free in $dir under ${min_disk}MB;"
  fi

  if [ -z "$why" ]; then
    [ "$quiet" -eq 1 ] || echo "wait-for-capacity: room (cpu ${cpu}%, mem ${mem}%, ${ncpu} cpus)" >&2
    exit 0
  fi
  if [ "$waited" -ge "$timeout" ]; then
    [ "$quiet" -eq 1 ] || echo "wait-for-capacity: still busy after ${waited}s:$why" >&2
    exit 3
  fi
  [ "$quiet" -eq 1 ] || echo "wait-for-capacity: busy:$why waiting ${interval}s" >&2
  sleep "$interval"
  waited=$((waited + interval))
done
