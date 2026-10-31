import { useEffect, useRef } from "react";
import type { ReactNode } from "react";

import styles from "./PageLayout.module.css";

/**
 * Page layout with the route-focus target (Blueprint §7.4): after a route
 * change, focus moves to the page's main heading. PageHeading owns the
 * focus move itself (focus on mount), so it works at every nesting depth
 * — no outlet-context plumbing. The heading is tabindex="-1" and its
 * programmatic focus ring is suppressed by the global style; it is not an
 * interactive element.
 */
export function PageLayout({ children }: { children: ReactNode }) {
  return <div className={styles.page}>{children}</div>;
}

export function PageHeading({ children }: { children: ReactNode }) {
  const headingRef = useRef<HTMLHeadingElement>(null);

  useEffect(() => {
    headingRef.current?.focus();
  }, []);

  return (
    <h1 ref={headingRef} tabIndex={-1} className={styles.heading}>
      {children}
    </h1>
  );
}

export function PageSection({ children }: { children: ReactNode }) {
  return <section className={styles.section}>{children}</section>;
}
