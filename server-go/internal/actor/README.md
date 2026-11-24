# Actor runtime

B3 provides a deterministic per-actor mailbox runtime:

- one serial execution loop per actor;
- bounded FIFO mailbox;
- actor and system shutdown;
- delayed message scheduling;
- actor lifecycle callbacks.

The runtime deliberately does not expose arbitrary shared-state callbacks. Gameplay
state must remain owned by its actor or map executor.
