// Tauri APIs replaced with stubs for web mode
const getVersion = async (): Promise<string> => '0.0.0'
const writeText = (text: string) => navigator.clipboard.writeText(text)
import { useEffect, useMemo, useRef, useState } from "react";
import { IdeSettingsImportDialog } from "@/features/file-system/components/ide-settings-import-dialog";
import { useToast } from "@/features/layout/contexts/toast-context";
import { useUpdater } from "@/features/settings/hooks/use-updater";
import { Button } from "@/components/ui/button";
import Command, {
  CommandEmpty,
  CommandHeader,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { matchesSearchQuery } from "@/utils/search-match";
import { SettingRow } from "../settings-section";

const REPORT_BUG_CHANNELS = [
  {
    id: "github",
    label: "GitHub",
    detail: "Open a bug report issue",
    url: "https://github.com/rabbyte/crowbar/issues/new",
  },
  {
    id: "email",
    label: "Email",
    detail: "Send a report to hello@crowbar.dev",
    url: "mailto:hello@crowbar.dev",
  },
] as const;

type ReportBugChannel = (typeof REPORT_BUG_CHANNELS)[number];

export const GeneralSettings = () => {
  const {
    available,
    checking,
    downloading,
    installing,
    error,
    updateInfo,
    downloadProgress,
    checkForUpdates,
    downloadAndInstall,
  } = useUpdater(false);
  const { showToast } = useToast();

  const [appVersion, setAppVersion] = useState<string>("");
  const [isImportDialogOpen, setIsImportDialogOpen] = useState(false);
  const [isReportBugDialogOpen, setIsReportBugDialogOpen] = useState(false);

  useEffect(() => {
    getVersion().then(setAppVersion);
  }, []);

  const handleCheckForUpdates = async () => {
    const hasUpdate = await checkForUpdates({ ignoreSuppression: true });
    if (!hasUpdate) {
      showToast({ message: "You're on the latest version", type: "success" });
    }
  };

  const buildBugReport = async () => {
    const version = await getVersion();
    const os = { platform: () => "web", version: () => "0.0.0" };
    const plat = os.platform();
    const ver = os.version();

    return `Environment\n\n- App: Crowbar ${version}\n- OS: ${plat} ${ver}\n\nProblem\n\nDescribe the issue here. Steps to reproduce, expected vs actual.\n`;
  };

  const handleReportBug = async (channel: ReportBugChannel) => {
    try {
      const openUrl = (url: string) => { window.open(url, "_blank") };
      const report = await buildBugReport();

      if (channel.id === "email") {
        await openUrl(
          `${channel.url}?subject=${encodeURIComponent("Crowbar bug report")}&body=${encodeURIComponent(report)}`,
        );
      } else {
        await writeText(report);
        await openUrl(channel.url);
        showToast({ message: "Report template copied", type: "success" });
      }

      setIsReportBugDialogOpen(false);
    } catch (err) {
      console.error("Failed to prepare bug report:", err);
      showToast({ message: "Failed to prepare bug report", type: "error" });
    }
  };

  return (
    <div className="space-y-4">
      <SettingRow
        label="Version"
        description="Check for updates and install the latest app version."
      >
        <div className="flex flex-wrap justify-end gap-2">
          {available ? (
            <Button
              onClick={downloadAndInstall}
              disabled={downloading || installing}
              variant="default"
              compact
            >
              {downloading
                ? "Downloading..."
                : installing
                  ? "Installing..."
                  : `Install ${updateInfo?.version ?? "update"}`}
            </Button>
          ) : (
            <Button
              onClick={handleCheckForUpdates}
              disabled={checking || downloading || installing}
              variant="default"
              compact
            >
              {checking ? "Checking..." : "Check"}
            </Button>
          )}
        </div>
      </SettingRow>

      <div className="ui-font ui-text-xs -mt-3 px-1 text-text-lighter/75">
        {downloading
          ? `Crowbar ${appVersion || "..."} · Downloading ${downloadProgress?.percentage ?? 0}%`
          : installing
            ? `Crowbar ${appVersion || "..."} · Installing update...`
            : available
              ? `Crowbar ${appVersion || "..."} · Version ${updateInfo?.version} available`
              : error
                ? `Crowbar ${appVersion || "..."} · Failed to check for updates`
                : `Crowbar ${appVersion || "..."} · App is up to date`}
      </div>

      {downloading && downloadProgress && (
        <div className="px-3">
          <div className="h-1 w-full overflow-hidden rounded-full bg-secondary-bg">
            <div
              className="h-full bg-accent transition-all duration-300"
              style={{ width: `${downloadProgress.percentage}%` }}
            />
          </div>
        </div>
      )}

      {error && <div className="ui-font ui-text-sm px-3 text-error">{error}</div>}

      <SettingRow label="Import Settings" description="Import matching setup from another editor.">
        <Button onClick={() => setIsImportDialogOpen(true)} variant="default" compact>
          Import
        </Button>
      </SettingRow>

      <SettingRow
        label="Report a Bug"
        description="Choose where to report an issue with environment details."
      >
        <Button onClick={() => setIsReportBugDialogOpen(true)} variant="default" compact>
          Open
        </Button>
      </SettingRow>

      {isImportDialogOpen && (
        <IdeSettingsImportDialog onClose={() => setIsImportDialogOpen(false)} />
      )}
      {isReportBugDialogOpen && (
        <ReportBugCommandDialog
          onClose={() => setIsReportBugDialogOpen(false)}
          onSelect={(channel) => void handleReportBug(channel)}
        />
      )}
    </div>
  );
};

function ReportBugCommandDialog({
  onClose,
  onSelect,
}: {
  onClose: () => void;
  onSelect: (channel: ReportBugChannel) => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const channels = useMemo(
    () =>
      REPORT_BUG_CHANNELS.filter((channel) =>
        matchesSearchQuery(query, [channel.label, channel.detail]),
      ),
    [query],
  );

  useEffect(() => {
    setSelectedIndex(0);
  }, [query]);

  const selectedChannel = channels[selectedIndex];

  const handleKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setSelectedIndex((index) => Math.min(index + 1, channels.length - 1));
      return;
    }

    if (event.key === "ArrowUp") {
      event.preventDefault();
      setSelectedIndex((index) => Math.max(index - 1, 0));
      return;
    }

    if (event.key === "Enter" && selectedChannel) {
      event.preventDefault();
      onSelect(selectedChannel);
    }
  };

  return (
    <Command isVisible onClose={onClose} title="Report a Bug" className="w-[520px]">
      <CommandHeader onClose={onClose}>
        <CommandInput
          ref={inputRef}
          value={query}
          onChange={setQuery}
          onKeyDown={handleKeyDown}
          placeholder="Report via..."
        />
      </CommandHeader>
      <CommandList>
        {channels.length === 0 ? (
          <CommandEmpty>No report channel matches "{query}".</CommandEmpty>
        ) : (
          channels.map((channel, index) => (
            <CommandItem
              key={channel.id}
              isSelected={index === selectedIndex}
              onClick={() => onSelect(channel)}
              onMouseEnter={() => setSelectedIndex(index)}
              className="h-8 items-center justify-between px-3"
            >
              <span className="ui-font ui-text-sm text-text">{channel.label}</span>
              <span className="ui-font ui-text-sm shrink-0 text-text-lighter">
                {channel.detail}
              </span>
            </CommandItem>
          ))
        )}
      </CommandList>
    </Command>
  );
}
