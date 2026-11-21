/**
 * Side-Pane `Side conversation` Isolated Thread (UIR-5.3, Blueprint §8.5).
 *
 * Scoped to the active workspace without mutating or polluting the primary task transcript.
 * Supports quick insertion of a side answer back into the main Agentic Composer.
 */

import { useState, type ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Badge, Button, Input } from "../components/primitives";
import type { SideConversationMessage } from "./sidePaneConfig";

export interface SideConversationTabProps {
  readonly workspaceName: string;
  readonly onInsertToMainComposer: (text: string) => void;
}

const INITIAL_SIDE_MESSAGES: readonly SideConversationMessage[] = [
  {
    messageId: "side-1",
    role: "assistant",
    text: "Side conversation is isolated from your primary task transcript. Ask quick workspace questions here or promote snippets to the main composer.",
    timestamp: "Ready",
  },
];

export function SideConversationTab({
  workspaceName,
  onInsertToMainComposer,
}: SideConversationTabProps): ReactElement {
  const [messages, setMessages] = useState<readonly SideConversationMessage[]>(
    INITIAL_SIDE_MESSAGES,
  );
  const [draft, setDraft] = useState("");

  const handleSend = () => {
    const trimmed = draft.trim();
    if (!trimmed) return;
    const nowId = Date.now();
    setMessages((prev) => [
      ...prev,
      {
        messageId: `side-u-${nowId}`,
        role: "user",
        text: trimmed,
        timestamp: "Just now",
      },
      {
        messageId: `side-a-${nowId + 1}`,
        role: "assistant",
        text: `Scoped note for ${workspaceName}: "${trimmed}" (isolated from main task transcript).`,
        timestamp: "Just now",
      },
    ]);
    setDraft("");
  };

  return (
    <section
      className="ky-side-convo"
      data-testid="side-conversation-tab"
      aria-label="Isolated side conversation"
    >
      <header className="ky-side-convo__header">
        <div className="ky-side-convo__title">
          <ChromeIcon name="sparkles" size={16} />
          <strong>Side conversation</strong>
          <Badge tone="neutral" label="Isolated" />
        </div>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => setMessages(INITIAL_SIDE_MESSAGES)}
        >
          Clear thread
        </Button>
      </header>

      <div className="ky-side-convo__messages">
        {messages.map((msg) => (
          <article
            key={msg.messageId}
            className="ky-side-convo__bubble"
            data-role={msg.role}
          >
            <p>{msg.text}</p>
            <div className="ky-side-convo__bubble-foot">
              <span>{msg.timestamp}</span>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => onInsertToMainComposer(msg.text)}
              >
                Insert into main composer
              </Button>
            </div>
          </article>
        ))}
      </div>

      <div className="ky-side-convo__composer">
        <Input
          aria-label="Side conversation prompt"
          placeholder="Ask a quick side question..."
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              handleSend();
            }
          }}
        />
        <Button size="sm" variant="primary" onClick={handleSend} aria-label="Send side question">
          <ChromeIcon name="send" size={14} />
        </Button>
      </div>
    </section>
  );
}
