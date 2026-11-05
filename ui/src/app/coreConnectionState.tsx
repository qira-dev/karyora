import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";

import {
  createHttpLocalTransport,
  type CoreSessionMetadata,
  type ExchangeOutcome,
  type HttpLocalTransport,
} from "../coreclient/httpLocal";

/**
 * The shell's Core-connection state (C-02; UIR-3's shell states), driven
 * by REAL transport outcomes only (IMPL-2).
 *
 * One transport instance for the app; the phase derives exclusively from
 * what the Core's local transport actually answers:
 *
 * - `connecting` — a probe is in flight and no prior state exists.
 * - `unknown` — no transport answered this page's origin. This is the
 *   existing truthful-unavailable state (the Core's local transport is
 *   opt-in and disabled by default; a static deployment has no proxy).
 *   Unknown is never shown as connected, and no state is invented.
 * - `disconnected` — a Core transport answered (the probe read received
 *   its coded `forbidden` denial) but no session is established, so no
 *   authorized operation can run yet.
 * - `connected` — a Core transport answered AND the held session
 *   authorized the probe read (any coded non-forbidden outcome).
 * - `reconnecting` — a prior state existed and a re-check is in flight.
 * - `session-expired` — a held session no longer authorizes the read
 *   (the coded `forbidden` outcome with a session previously held). The
 *   token has been discarded from memory; the safe metadata snapshot is
 *   kept for the truthful display only.
 *
 * Honest divergences (recorded, never hidden): the designed `stale`
 * state has no real trigger in this milestone — no data surface consumes
 * events yet (the Jobs surfaces are a later package), so nothing is
 * stale and nothing fabricates it. Phases change ONLY on real outcomes:
 * no timers, no optimistic transitions, no fabricated states.
 */

export type CoreConnectionPhase =
  | "connecting"
  | "unknown"
  | "disconnected"
  | "connected"
  | "reconnecting"
  | "session-expired";

interface CoreConnectionContextValue {
  readonly phase: CoreConnectionPhase;
  /** Safe display metadata of the current or last-held session; never a
   *  token. Null when no session was ever established. */
  readonly session: CoreSessionMetadata | null;
  /** Re-runs the probe (a real transport outcome, never optimism). */
  refresh: () => void;
  /** The C-1 exchange with the pasted one-time credential. */
  exchange: (credential: string) => Promise<ExchangeOutcome>;
  /** The deliberate logout's confirmed transition (IMPL-23): the
   *  dedicated logout flow calls this ONLY on the Core's confirmed 200 —
   *  the real outcome the transition derives from (never optimism: the
   *  response proves the session is dead, and a probe re-check reports
   *  the same disconnected posture for a held-nothing client). The token
   *  is discarded from memory and the phase renders the honest
   *  no-session state. */
  signedOut: () => void;
  /** The ONE transport instance for the app. The Jobs surfaces (IMPL-5)
   *  consume it for their reads and the cancellation command; every
   *  outcome they render comes from this instance's real transport
   *  results. The session secret stays inside the holder — this object
   *  exposes no token, and no method here retries on its own. */
  transport: HttpLocalTransport;
}

const CoreConnectionContext = createContext<CoreConnectionContextValue | undefined>(undefined);

export function CoreConnectionProvider({ children }: { children: ReactNode }) {
  const transportRef = useRef<HttpLocalTransport | null>(null);
  if (transportRef.current === null) {
    transportRef.current = createHttpLocalTransport();
  }
  const transport = transportRef.current;

  const [phase, setPhase] = useState<CoreConnectionPhase>("connecting");
  const [session, setSession] = useState<CoreSessionMetadata | null>(null);
  // The last settled phase, so a re-check can render `reconnecting`
  // instead of a fabricated reset to `connecting`.
  const settledRef = useRef<CoreConnectionPhase | null>(null);

  const runProbe = useCallback(
    (inFlightPhase: CoreConnectionPhase) => {
      setPhase(inFlightPhase);
      void transport.probe().then((result) => {
        if (result.kind === "unreachable") {
          settledRef.current = "unknown";
          setPhase("unknown");
          return;
        }
        if (result.outcome === "forbidden") {
          // The read was denied by the decision port: either no session
          // is held, or the held one no longer authorizes.
          const lastSettled = settledRef.current;
          if (isSessionHeldPhase(lastSettled)) {
            const metadata = transport.discardSession();
            setSession(metadata);
            settledRef.current = "session-expired";
            setPhase("session-expired");
          } else {
            settledRef.current = "disconnected";
            setPhase("disconnected");
          }
          return;
        }
        settledRef.current = "connected";
        setPhase("connected");
      });
    },
    [transport],
  );

  // One probe on mount: a single real read that establishes whether a
  // Core transport answers this origin, and whether the held session (if
  // any) still authorizes. No polling loop exists in this milestone.
  useEffect(() => {
    runProbe("connecting");
  }, [runProbe]);

  const refresh = useCallback(() => {
    const prior = settledRef.current;
    runProbe(prior === null || prior === "connecting" ? "connecting" : "reconnecting");
  }, [runProbe]);

  const signedOut = useCallback(() => {
    // The deliberate logout's confirmed transition: the calling surface
    // ran the C-4 logout command and received the Core's confirmed 200 —
    // the real outcome this derives from. The token is discarded (the
    // holder's law) and the honest no-session posture renders; a probe
    // re-check reports the same disconnected state for a held-nothing
    // client, so nothing here is assumed.
    const metadata = transport.discardSession();
    setSession(metadata);
    settledRef.current = "disconnected";
    setPhase("disconnected");
  }, [transport]);

  const exchange = useCallback(
    async (credential: string): Promise<ExchangeOutcome> => {
      const outcome = await transport.exchange(credential);
      if (outcome.kind === "data") {
        setSession(outcome.data);
        // The probe read with the new session makes `connected` a
        // verified state, not an assumed one.
        runProbe("reconnecting");
      } else if (outcome.kind === "rejection") {
        // The Core answered with a typed refusal: the transport is
        // demonstrably reachable and no session was established.
        settledRef.current = "disconnected";
        setPhase("disconnected");
      } else {
        // Lost/uninterpretable: re-check against a real outcome.
        refresh();
      }
      return outcome;
    },
    [transport, runProbe, refresh],
  );

  const value = useMemo<CoreConnectionContextValue>(
    () => ({ phase, session, refresh, exchange, signedOut, transport }),
    [phase, session, refresh, exchange, signedOut, transport],
  );

  return <CoreConnectionContext.Provider value={value}>{children}</CoreConnectionContext.Provider>;
}

function isSessionHeldPhase(phase: CoreConnectionPhase | null): boolean {
  return phase === "connected" || phase === "session-expired";
}

export function useCoreConnection(): CoreConnectionContextValue {
  const context = useContext(CoreConnectionContext);
  if (!context) throw new Error("useCoreConnection must be used inside CoreConnectionProvider");
  return context;
}
