# Dashboard Improvements

**Date:** 2026-04-01
**Status:** Planned (not started)

## Current State

The dashboard at `/dashboard` is a basic static HTML page with client-side JavaScript. It shows boards and tasks but lacks analytics and visualisation.

## Planned Improvements

- **Cumulative flow diagram (CFD)**: stacked area chart of task counts per state over time, derived from audit log
- **Cycle time report**: time from first non-initial state to terminal state per task, as histogram and averages
- **Throughput report**: tasks completed per time period (day/week/month)
- **Actor activity charts**: visual breakdown of activity by actor over time
- **Board health indicators**: per-board summary with WIP counts, stale tasks, blocked items
