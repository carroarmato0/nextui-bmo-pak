# Profiling Findings — bmo-pak (YYYY-MM-DD)

## Environment

- Device / platform: (tg5040 Brick / Smart Pro | tg5050 Smart Pro S)
- Framebuffer resolution:
- Build / commit:
- Flags used:

## Workloads exercised

| Workload            | uptime_s range | Notes |
|---------------------|----------------|-------|
| idle                |                |       |
| idle animations     |                |       |
| listening + thinking|                |       |
| speaking            |                |       |

## CPU findings

(`go tool pprof -top` highlights; hot functions and why.)

## Heap findings (Go allocations)

(`inuse_space` top; notable Go-side allocators.)

## RSS-over-time findings (whole process)

- Peak VmRSS: __ kB during state: __
- Non-Go footprint (VmRSS − HeapSys): __ kB — i.e. SDL textures / CGO.
- Leak check (RSS across repeated same-state cycles): (stable / growing)

## Action Items (ranked by impact)

1. **[highest impact]** — evidence: (func / line / CSV observation). Expected win:
2.
3.
