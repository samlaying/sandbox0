"use client";

import React, { useState, useEffect } from "react";
import { cn } from "@sandbox0/ui";
import { PixelCodeBlock } from "./PixelCodeBlock";

const STORAGE_KEY = "sandbox0-docs-preferred-tab";
const SYNC_EVENT = "sandbox0-tabs-sync";

export interface PixelTab {
  label: string;
  /** React node content (takes precedence if provided) */
  content?: React.ReactNode;
  /** Code content for code tabs (will be rendered with syntax highlighting) */
  code?: string;
  /** Language for syntax highlighting (e.g., 'go', 'python', 'typescript', 'bash') */
  language?: string;
}

export interface PixelTabsProps {
  tabs: PixelTab[];
  defaultTab?: number;
  className?: string;
}

/**
 * PixelTabs - Tabbed interface for code examples and content variants
 * Features global synchronization via localStorage and custom events
 */
export function PixelTabs({
  tabs,
  defaultTab = 0,
  className,
}: PixelTabsProps) {
  const [activeTab, setActiveTab] = useState(defaultTab);

  // Initialize from localStorage and handle sync
  useEffect(() => {
    const savedLabel = localStorage.getItem(STORAGE_KEY);
    if (savedLabel) {
      const idx = tabs.findIndex((t) => t.label === savedLabel);
      if (idx !== -1) {
        setActiveTab(idx);
      }
    }

    const handleSync = (e: CustomEvent | StorageEvent) => {
      let label: string | null = null;
      if (e instanceof CustomEvent) {
        label = e.detail;
      } else if (e instanceof StorageEvent && e.key === STORAGE_KEY) {
        label = e.newValue;
      }

      if (label) {
        const idx = tabs.findIndex((t) => t.label === label);
        if (idx !== -1) {
          setActiveTab(idx);
        }
      }
    };

    window.addEventListener(SYNC_EVENT as any, handleSync);
    window.addEventListener("storage", handleSync);

    return () => {
      window.removeEventListener(SYNC_EVENT as any, handleSync);
      window.removeEventListener("storage", handleSync);
    };
  }, [tabs]);

  const handleTabChange = (idx: number) => {
    const label = tabs[idx].label;
    setActiveTab(idx);
    localStorage.setItem(STORAGE_KEY, label);
    
    // Dispatch event for same-page sync
    window.dispatchEvent(new CustomEvent(SYNC_EVENT, { detail: label }));
  };

  // Render tab content with code block support
  const renderTabContent = (tab: PixelTab) => {
    if (tab.content) {
      return tab.content;
    }
    if (tab.code) {
      return (
        <PixelCodeBlock language={tab.language}>
          {tab.code}
        </PixelCodeBlock>
      );
    }
    return null;
  };

  return (
    <div className={cn("w-full", className)}>
      {/* Tab Headers */}
      <div className="flex border-b border-foreground/15">
        {tabs.map((tab, idx) => (
          <button
            key={idx}
            onClick={() => handleTabChange(idx)}
            className={cn(
              "px-4 py-2 text-sm font-mono transition-all",
              "border-r border-foreground/15 last:border-r-0",
              activeTab === idx
                ? "bg-accent text-white"
                : "bg-surface text-muted hover:text-foreground hover:bg-surface/80"
            )}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Tab Content */}
      <div className="mt-0">{renderTabContent(tabs[activeTab])}</div>
    </div>
  );
}
