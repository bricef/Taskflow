# TUI Improvements

**Date:** 2026-03-31
**Status:** Planned (not started)
**Prerequisite:** None

## 1. Page-down scrolling in events, workflow, and list views

The events panel, workflow view, and task list views should support page-down/page-up scrolling (PgDn/PgUp or Ctrl-D/Ctrl-U) for navigating long content without holding an arrow key.

## 2. Searchable transition and assignment selectors

When performing a transition or assignment action, the list of options (states or actors) should be filterable by typing. This is important as the number of workflow states or team members grows.

## 3. @me shortcut for assignment

Support `@me` as a shortcut in the assignment selector to quickly assign a task to the currently authenticated user, matching the convention already used in the API and CLI.

## 4. "Take" action for self-assignment

Add a `take` action that assigns the selected task to the current user in a single keystroke. Should be available from the kanban board, task list, and task detail views. Equivalent to assign + @me but faster for the common case.
