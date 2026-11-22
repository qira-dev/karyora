/**
 * Interactive Whiteboard Tab (1:1 ZCode v3.18.2 `whiteboard` side-pane tab).
 *
 * Supports Pen, Eraser, Color swatches, Stroke width, Undo, Redo, Clear, and Add to chat.
 */

import { useEffect, useRef, useState, type PointerEvent, type ReactElement } from "react";
import { ChromeIcon } from "../components/icons";
import { Button } from "../components/primitives";

export interface WhiteboardTabProps {
  readonly onAddToChat: (excerpt: string, sourceLabel: string) => void;
}

interface StrokePath {
  readonly tool: "pen" | "eraser";
  readonly color: string;
  readonly width: number;
  readonly points: readonly { readonly x: number; readonly y: number }[];
}

const SWATCH_COLORS = ["#f0f0f0", "#38bdf8", "#4ade80", "#fbbf24", "#f87171", "#a78bfa"] as const;

export function WhiteboardTab({ onAddToChat }: WhiteboardTabProps): ReactElement {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const [boardName, setBoardName] = useState("Whiteboard");
  const [tool, setTool] = useState<"pen" | "eraser">("pen");
  const [color, setColor] = useState<string>(SWATCH_COLORS[1]);
  const [strokeWidth, setStrokeWidth] = useState(3);
  const [strokes, setStrokes] = useState<readonly StrokePath[]>([]);
  const [redoStack, setRedoStack] = useState<readonly StrokePath[]>([]);
  const activeStrokeRef = useRef<StrokePath | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    ctx.clearRect(0, 0, canvas.width, canvas.height);
    for (const s of strokes) {
      if (s.points.length < 2) continue;
      ctx.save();
      ctx.lineCap = "round";
      ctx.lineJoin = "round";
      ctx.lineWidth = s.width;
      if (s.tool === "eraser") {
        ctx.globalCompositeOperation = "destination-out";
        ctx.strokeStyle = "rgba(0,0,0,1)";
      } else {
        ctx.globalCompositeOperation = "source-over";
        ctx.strokeStyle = s.color;
      }
      ctx.beginPath();
      ctx.moveTo(s.points[0].x, s.points[0].y);
      for (let i = 1; i < s.points.length; i += 1) {
        ctx.lineTo(s.points[i].x, s.points[i].y);
      }
      ctx.stroke();
      ctx.restore();
    }
  }, [strokes]);

  const toCanvasPoint = (e: PointerEvent<HTMLCanvasElement>) => {
    const canvas = canvasRef.current;
    if (!canvas) return { x: 0, y: 0 };
    const rect = canvas.getBoundingClientRect();
    const scaleX = canvas.width / Math.max(1, rect.width);
    const scaleY = canvas.height / Math.max(1, rect.height);
    return {
      x: (e.clientX - rect.left) * scaleX,
      y: (e.clientY - rect.top) * scaleY,
    };
  };

  const handlePointerDown = (e: PointerEvent<HTMLCanvasElement>) => {
    e.currentTarget.setPointerCapture(e.pointerId);
    const pt = toCanvasPoint(e);
    const next: StrokePath = {
      tool,
      color,
      width: tool === "eraser" ? strokeWidth * 4 : strokeWidth,
      points: [pt],
    };
    activeStrokeRef.current = next;
    setRedoStack([]);
    setStrokes((prev) => [...prev, next]);
  };

  const handlePointerMove = (e: PointerEvent<HTMLCanvasElement>) => {
    const current = activeStrokeRef.current;
    if (!current) return;
    const pt = toCanvasPoint(e);
    const updated: StrokePath = {
      ...current,
      points: [...current.points, pt],
    };
    activeStrokeRef.current = updated;
    setStrokes((prev) => [...prev.slice(0, -1), updated]);
  };

  const handlePointerUp = () => {
    activeStrokeRef.current = null;
  };

  const handleUndo = () => {
    setStrokes((prev) => {
      if (prev.length === 0) return prev;
      const last = prev[prev.length - 1];
      setRedoStack((r) => [...r, last]);
      return prev.slice(0, -1);
    });
  };

  const handleRedo = () => {
    setRedoStack((prev) => {
      if (prev.length === 0) return prev;
      const last = prev[prev.length - 1];
      setStrokes((s) => [...s, last]);
      return prev.slice(0, -1);
    });
  };

  return (
    <section
      className="ky-whiteboard-tab"
      data-testid="whiteboard-tab"
      aria-label="Interactive whiteboard"
    >
      <header className="ky-whiteboard-toolbar">
        <input
          type="text"
          aria-label="Whiteboard name"
          className="ky-whiteboard-name"
          value={boardName}
          onChange={(e) => setBoardName(e.target.value)}
        />

        <div className="ky-whiteboard-controls">
          <Button
            size="sm"
            variant={tool === "pen" ? "secondary" : "ghost"}
            selected={tool === "pen"}
            aria-label="Pen"
            title="Pen"
            onClick={() => setTool("pen")}
          >
            <ChromeIcon name="pen-tool" size={14} />
          </Button>
          <Button
            size="sm"
            variant={tool === "eraser" ? "secondary" : "ghost"}
            selected={tool === "eraser"}
            aria-label="Eraser"
            title="Eraser"
            onClick={() => setTool("eraser")}
          >
            <ChromeIcon name="eraser" size={14} />
          </Button>

          <div className="ky-whiteboard-swatches" role="group" aria-label="Color">
            {SWATCH_COLORS.map((hex) => (
              <button
                key={hex}
                type="button"
                className="ky-whiteboard-swatch"
                data-active={color === hex && tool === "pen" ? "true" : "false"}
                style={{ backgroundColor: hex }}
                aria-label={`Select color ${hex}`}
                onClick={() => {
                  setTool("pen");
                  setColor(hex);
                }}
              />
            ))}
          </div>

          <input
            type="range"
            aria-label="Stroke width"
            min={1}
            max={12}
            value={strokeWidth}
            onChange={(e) => setStrokeWidth(Number(e.target.value))}
            style={{ width: "68px" }}
          />

          <Button
            size="sm"
            variant="ghost"
            disabled={strokes.length === 0}
            aria-label="Undo"
            title="Undo"
            onClick={handleUndo}
          >
            <ChromeIcon name="undo" size={14} />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={redoStack.length === 0}
            aria-label="Redo"
            title="Redo"
            onClick={handleRedo}
          >
            <ChromeIcon name="redo" size={14} />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={strokes.length === 0}
            onClick={() => {
              setStrokes([]);
              setRedoStack([]);
            }}
          >
            Clear
          </Button>
          <Button
            size="sm"
            variant="primary"
            onClick={() =>
              onAddToChat(
                `[Whiteboard Sketch: "${boardName}" — ${strokes.length} vector stroke(s) attached for visual reference]`,
                `whiteboard/${boardName.toLowerCase().replace(/\s+/g, "-")}.svg`,
              )
            }
          >
            Add to chat
          </Button>
        </div>
      </header>

      <div className="ky-whiteboard-stage">
        <canvas
          ref={canvasRef}
          width={960}
          height={640}
          aria-label="Whiteboard canvas"
          className="ky-whiteboard-canvas"
          onPointerDown={handlePointerDown}
          onPointerMove={handlePointerMove}
          onPointerUp={handlePointerUp}
          onPointerLeave={handlePointerUp}
        />
      </div>
    </section>
  );
}

export default WhiteboardTab;
