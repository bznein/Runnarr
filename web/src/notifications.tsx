import { useEffect, useState } from "react";
import { Bell, CheckCircle2, ChevronDown, ChevronUp, CircleAlert, ExternalLink, Info, Send, TriangleAlert, Trash2 } from "lucide-react";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocation, useNavigate } from "react-router-dom";
import { api } from "./api";
import type { NotificationCategory, NotificationMode, NotificationSeverity, PushSubscriptionDevice, RunnarrNotification } from "./types";

const categoryLabels: Record<NotificationCategory, { title: string; description: string }> = {
  workout_changes: { title: "Workout changes", description: "Generated, updated, removed, or parse warnings." },
  garmin_calendar: { title: "Garmin calendar", description: "Scheduling, removal, conflicts, and recovery." },
  activity_matching: { title: "Activity matching", description: "Completed activities automatically matched to the plan." },
  sheet_writeback: { title: "Sheet writeback", description: "Writeback failures, partial results, and recovery." }
};

const modeLabels: Record<NotificationMode, string> = {
  off: "Off",
  in_app: "In app",
  in_app_push: "In app + push"
};

function notificationDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}

function NotificationSeverityMarker({ severity }: { severity: NotificationSeverity }) {
  const Icon = severity === "success"
    ? CheckCircle2
    : severity === "warning"
      ? TriangleAlert
      : severity === "error"
        ? CircleAlert
        : Info;
  return <span className={`notification-severity ${severity}`} title={`${severity} notification`}><Icon size={16} aria-hidden="true" /></span>;
}

async function invalidateNotifications(queryClient: ReturnType<typeof useQueryClient>) {
  await queryClient.invalidateQueries({ queryKey: ["notifications"] });
}

export function NotificationBell({ mobile = false, canWrite = true }: { mobile?: boolean; canWrite?: boolean }) {
  const [open, setOpen] = useState(false);
  const [markAllFeedback, setMarkAllFeedback] = useState("");
  const location = useLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const notifications = useQuery({
    queryKey: ["notifications", "recent", "unread"],
    queryFn: () => api.notifications({ limit: 5, unread: true }),
    refetchInterval: 30_000,
    refetchOnWindowFocus: true
  });
  useEffect(() => setOpen(false), [location.pathname, location.search]);
  useEffect(() => {
    if (!open) setMarkAllFeedback("");
  }, [open]);
  useEffect(() => updateAppBadge(notifications.data?.unreadCount), [notifications.data?.unreadCount]);
  const markAll = useMutation({
    mutationFn: api.markAllNotificationsRead,
    onMutate: () => setMarkAllFeedback(""),
    onSuccess: async () => {
      setMarkAllFeedback("All notifications marked as read.");
      await invalidateNotifications(queryClient);
    }
  });
  const openNotification = async (item: RunnarrNotification) => {
    if (canWrite && !item.readAt) {
      await api.setNotificationRead(item.id, true).catch(() => undefined);
      void invalidateNotifications(queryClient);
    }
    navigate(safeNotificationActionPath(item.actionPath));
  };
  const unreadCount = notifications.data?.unreadCount ?? 0;
  return (
    <div className={`notification-bell ${mobile ? "mobile-notification-bell" : ""}`}>
      <button className={mobile ? "icon-button" : "nav-button notification-nav-button"} type="button" aria-label={`${unreadCount} unread notifications`} aria-expanded={open} onClick={() => setOpen((value) => !value)}>
        <Bell size={18} />
        {!mobile && <span>Notifications</span>}
        {unreadCount > 0 && <span className="notification-badge">{unreadCount > 99 ? "99+" : unreadCount}</span>}
      </button>
      {open && (
        <div className="notification-popover" role="dialog" aria-label="Recent notifications">
          <div className="notification-popover-heading"><strong>Notifications</strong>{canWrite && unreadCount > 0 && <button className="text-button" type="button" disabled={markAll.isPending} onClick={() => markAll.mutate()}>{markAll.isPending ? "Marking…" : "Mark all read"}</button>}</div>
          <div className="notification-popover-list">
            {(notifications.data?.notifications ?? []).map((item) => <button key={item.id} className={`notification-popover-item ${item.readAt ? "" : "unread"}`} type="button" onClick={() => void openNotification(item)}><NotificationSeverityMarker severity={item.severity} /><span><strong>{item.title}</strong>{item.body && <small>{item.body}</small>}<time>{notificationDate(item.lastEventAt)}</time></span></button>)}
            {notifications.error && <div className="notification-empty error-text">Could not load notifications.</div>}
            {!notifications.error && !notifications.isLoading && (notifications.data?.notifications.length ?? 0) === 0 && <div className="notification-empty">No unread notifications.</div>}
          </div>
          {markAllFeedback && <div className="notification-action-feedback" role="status" aria-live="polite">{markAllFeedback}</div>}
          {markAll.error && <div className="notification-action-error" role="alert">{markAll.error instanceof Error ? markAll.error.message : "Could not mark notifications as read."}</div>}
          <button className="notification-view-all" type="button" onClick={() => navigate("/notifications")}>View all notifications</button>
        </div>
      )}
    </div>
  );
}

export function NotificationsPage({ canWrite = true }: { canWrite?: boolean }) {
  const [unreadOnly, setUnreadOnly] = useState(false);
  const [expanded, setExpanded] = useState<string>();
  const [bulkFeedback, setBulkFeedback] = useState("");
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const list = useInfiniteQuery({
    queryKey: ["notifications", "page", unreadOnly],
    queryFn: ({ pageParam }) => api.notifications({ limit: 20, cursor: pageParam, unread: unreadOnly }),
    initialPageParam: "",
    getNextPageParam: (page) => page.nextCursor || undefined
  });
  const items = list.data?.pages.flatMap((page) => page.notifications) ?? [];
  const unreadCount = list.data?.pages[0]?.unreadCount ?? 0;
  const detail = useQuery({ queryKey: ["notifications", "detail", expanded], queryFn: () => api.notification(expanded!), enabled: Boolean(expanded) });
  const refresh = () => invalidateNotifications(queryClient);
  const setRead = useMutation({ mutationFn: ({ id, read }: { id: string; read: boolean }) => api.setNotificationRead(id, read), onSuccess: refresh });
  const remove = useMutation({ mutationFn: api.deleteNotification, onSuccess: refresh });
  const markAll = useMutation({
    mutationFn: api.markAllNotificationsRead,
    onMutate: () => setBulkFeedback(""),
    onSuccess: async () => {
      setBulkFeedback("All notifications marked as read.");
      await refresh();
    }
  });
  const clear = useMutation({
    mutationFn: api.clearNotifications,
    onMutate: () => setBulkFeedback(""),
    onSuccess: async (_result, scope) => {
      setBulkFeedback(scope === "read" ? "Read notifications cleared." : "All notifications cleared.");
      await refresh();
    }
  });
  const bulkError = markAll.error || clear.error;
  const clearScope = (scope: "read" | "all") => {
    if (window.confirm(scope === "all" ? "Clear all notification history?" : "Clear all read notifications?")) clear.mutate(scope);
  };
  return (
    <div className="page notifications-page">
      <header className="page-header"><div className="page-title"><h1>Notifications</h1><p>Workout, Garmin, matching, and writeback events that need your attention.</p></div></header>
      <div className="notification-toolbar">
        <div className="segmented-control"><button className={!unreadOnly ? "active" : ""} type="button" onClick={() => setUnreadOnly(false)}>All</button><button className={unreadOnly ? "active" : ""} type="button" onClick={() => setUnreadOnly(true)}>Unread</button></div>
        {canWrite && <div className="notification-toolbar-actions"><button className="secondary-button small-button" type="button" disabled={unreadCount === 0 || markAll.isPending} onClick={() => markAll.mutate()}>{markAll.isPending ? "Marking…" : "Mark all read"}</button><button className="secondary-button small-button" type="button" disabled={clear.isPending} onClick={() => clearScope("read")}>{clear.isPending ? "Clearing…" : "Clear read"}</button><button className="danger-button small-button" type="button" disabled={clear.isPending} onClick={() => clearScope("all")}>Clear all</button></div>}
      </div>
      {bulkFeedback && <div className="notification-action-feedback panel" role="status" aria-live="polite">{bulkFeedback}</div>}
      {bulkError && <div className="error" role="alert">{bulkError instanceof Error ? bulkError.message : "Could not update notifications."}</div>}
      <section className="notification-list">
        {items.map((item) => <article key={item.id} className={`panel notification-card ${item.readAt ? "" : "unread"}`}>
          <button className="notification-card-main" type="button" onClick={() => setExpanded((value) => value === item.id ? undefined : item.id)}><NotificationSeverityMarker severity={item.severity} /><span className="notification-card-copy"><strong>{item.title}</strong>{item.body && <span>{item.body}</span>}<time>{notificationDate(item.lastEventAt)} · {item.eventCount} {item.eventCount === 1 ? "event" : "events"}</time></span>{expanded === item.id ? <ChevronUp size={18} /> : <ChevronDown size={18} />}</button>
          {expanded === item.id && <div className="notification-timeline">
            {(detail.data?.events ?? []).map((event) => <div key={event.id} className="notification-timeline-event"><NotificationSeverityMarker severity={event.severity} /><div><strong>{event.title}</strong>{event.body && <p>{event.body}</p>}<time>{notificationDate(event.createdAt)}</time></div></div>)}
            {detail.error && <div className="error">{detail.error instanceof Error ? detail.error.message : "Could not load notification history"}</div>}
            <div className="notification-card-actions"><button className="primary-button small-button" type="button" onClick={() => { if (canWrite && !item.readAt) setRead.mutate({ id: item.id, read: true }); navigate(safeNotificationActionPath(item.actionPath)); }}><ExternalLink size={14} />Open</button>{canWrite && <button className="secondary-button small-button" type="button" onClick={() => setRead.mutate({ id: item.id, read: !item.readAt })}>{item.readAt ? "Mark unread" : "Mark read"}</button>}{canWrite && <button className="icon-button danger" type="button" aria-label="Delete notification" onClick={() => remove.mutate(item.id)}><Trash2 size={15} /></button>}</div>
          </div>}
        </article>)}
        {list.error && <div className="error">{list.error instanceof Error ? list.error.message : "Could not load notifications"}</div>}
        {!list.error && !list.isLoading && items.length === 0 && <div className="panel notification-empty">{unreadOnly ? "No unread notifications." : "Nothing to review yet."}</div>}
      </section>
      {list.hasNextPage && <button className="secondary-button" type="button" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>{list.isFetchingNextPage ? "Loading…" : "Load more"}</button>}
    </div>
  );
}

export function NotificationSettingsSection({ canWrite = true }: { canWrite?: boolean }) {
  const queryClient = useQueryClient();
  const settings = useQuery({ queryKey: ["notification-settings"], queryFn: api.notificationSettings });
  const devices = useQuery({ queryKey: ["push-subscriptions"], queryFn: api.pushSubscriptions, enabled: canWrite });
  const [deviceName, setDeviceName] = useState(defaultDeviceName());
  const [permission, setPermission] = useState<NotificationPermission>(() => typeof Notification === "undefined" ? "denied" : Notification.permission);
  const [currentDeviceID, setCurrentDeviceID] = useState(() => window.localStorage.getItem("runnarrPushSubscriptionId") ?? "");
  const supported = pushSupported();

  useEffect(() => {
    if (currentDeviceID && devices.data && !devices.data.subscriptions.some((device) => device.id === currentDeviceID)) {
      window.localStorage.removeItem("runnarrPushSubscriptionId");
      setCurrentDeviceID("");
    }
  }, [currentDeviceID, devices.data]);

  const saveModes = useMutation({ mutationFn: api.updateNotificationSettings, onSuccess: () => queryClient.invalidateQueries({ queryKey: ["notification-settings"] }) });
  const enable = useMutation({ mutationFn: async () => enablePush(settings.data?.vapidPublicKey ?? "", deviceName), onSuccess: async (device) => { setCurrentDeviceID(device.id); setPermission(Notification.permission); window.localStorage.setItem("runnarrPushSubscriptionId", device.id); await queryClient.invalidateQueries({ queryKey: ["push-subscriptions"] }); } });
  const remove = useMutation({ mutationFn: api.deletePushSubscription, onSuccess: async (_, id) => { if (id === currentDeviceID) await unsubscribeBrowserPush(); await queryClient.invalidateQueries({ queryKey: ["push-subscriptions"] }); } });
  const test = useMutation({ mutationFn: api.testPushSubscription, onSuccess: () => window.alert("Test notification sent.") });
  const rename = async (device: PushSubscriptionDevice) => { const name = window.prompt("Device name", device.deviceName)?.trim(); if (!name) return; await api.renamePushSubscription(device.id, name); await queryClient.invalidateQueries({ queryKey: ["push-subscriptions"] }); };
  const updateMode = (category: NotificationCategory, mode: NotificationMode) => { if (settings.data) saveModes.mutate({ ...settings.data.categories, [category]: mode }); };
  const mutationError = settings.error || devices.error || enable.error || test.error || remove.error || saveModes.error;
  return (
    <section id="notifications" className="panel notification-settings-panel">
      <div><div className="panel-heading">Notifications</div><p className="muted">Choose which events enter the inbox and which also reach enabled devices.</p></div>
      <div className="notification-category-settings">{(Object.keys(categoryLabels) as NotificationCategory[]).map((category) => <label key={category} className="notification-category-row"><span><strong>{categoryLabels[category].title}</strong><small>{categoryLabels[category].description}</small></span><select disabled={!canWrite || saveModes.isPending} value={settings.data?.categories[category] ?? "in_app"} onChange={(event) => updateMode(category, event.target.value as NotificationMode)}>{(Object.keys(modeLabels) as NotificationMode[]).map((mode) => <option key={mode} value={mode}>{modeLabels[mode]}</option>)}</select></label>)}</div>
      {canWrite && <div className="push-device-settings">
        <div><strong>Browser push devices</strong><p className="muted">Permission is requested only when you enable this device.</p></div>
        {!supported && <div className="workout-notice">Push is unavailable here. On iPhone or iPad, add Runnarr to the Home Screen and open the installed app.</div>}
        {supported && permission === "denied" && <div className="workout-notice">Browser notifications are blocked. Allow them in this device’s site or notification settings.</div>}
        {supported && !currentDeviceID && <div className="push-enable-row"><input value={deviceName} maxLength={100} aria-label="Device name" onChange={(event) => setDeviceName(event.target.value)} /><button className="primary-button" type="button" disabled={!settings.data?.vapidPublicKey || !deviceName.trim() || enable.isPending} onClick={() => enable.mutate()}><Bell size={16} />{enable.isPending ? "Enabling…" : "Enable this device"}</button></div>}
        <div className="push-device-list">{(devices.data?.subscriptions ?? []).map((device) => <div key={device.id} className="push-device-row"><div><strong>{device.deviceName}{device.id === currentDeviceID ? " · This device" : ""}</strong><small>{device.lastSuccessAt ? `Last delivered ${notificationDate(device.lastSuccessAt)}` : "No successful delivery yet"}</small>{device.lastError && <small className="error-text">{device.lastError}</small>}</div><div><button className="secondary-button small-button" type="button" onClick={() => void rename(device)}>Rename</button>{device.id === currentDeviceID && <button className="secondary-button small-button" type="button" disabled={test.isPending} onClick={() => test.mutate(device.id)}><Send size={14} />Test</button>}<button className="icon-button danger" type="button" aria-label={`Remove ${device.deviceName}`} onClick={() => remove.mutate(device.id)}><Trash2 size={15} /></button></div></div>)}</div>
        {mutationError && <div className="error">{mutationError instanceof Error ? mutationError.message : "Could not update notifications"}</div>}
      </div>}
    </section>
  );
}

export async function unregisterCurrentPushDevice() {
  if (typeof window === "undefined") return;
  const currentDeviceID = window.localStorage.getItem("runnarrPushSubscriptionId") ?? "";
  if (!pushSupported()) {
    if (currentDeviceID) await api.deletePushSubscription(currentDeviceID).catch(() => undefined);
    window.localStorage.removeItem("runnarrPushSubscriptionId");
    return;
  }
  const registration = await navigator.serviceWorker.getRegistration();
  if (!registration) {
    if (currentDeviceID) await api.deletePushSubscription(currentDeviceID).catch(() => undefined);
    window.localStorage.removeItem("runnarrPushSubscriptionId");
    return;
  }
  const subscription = await registration.pushManager.getSubscription();
  if (subscription) {
    await api.deleteCurrentPushSubscription(subscription.endpoint).catch(async () => {
      if (currentDeviceID) await api.deletePushSubscription(currentDeviceID).catch(() => undefined);
    });
    await subscription.unsubscribe();
  } else if (currentDeviceID) {
    await api.deletePushSubscription(currentDeviceID).catch(() => undefined);
  }
  window.localStorage.removeItem("runnarrPushSubscriptionId");
}

function pushSupported() {
  return typeof window !== "undefined" && window.isSecureContext && "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;
}

async function enablePush(publicKey: string, deviceName: string) {
  if (!pushSupported() || !publicKey) throw new Error("Browser push is unavailable.");
  const permission = await Notification.requestPermission();
  if (permission !== "granted") throw new Error("Notification permission was not granted.");
  const registration = await notificationServiceWorkerRegistration();
  let subscription = await registration.pushManager.getSubscription();
  if (!subscription) subscription = await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: decodeApplicationServerKey(publicKey) });
  const json = subscription.toJSON();
  return api.createPushSubscription({ endpoint: json.endpoint!, expirationTime: json.expirationTime, keys: json.keys!, deviceName: deviceName.trim() });
}

async function unsubscribeBrowserPush() {
  if (!pushSupported()) return;
  const registration = await navigator.serviceWorker.getRegistration();
  if (!registration) {
    window.localStorage.removeItem("runnarrPushSubscriptionId");
    return;
  }
  const subscription = await registration.pushManager.getSubscription();
  if (subscription) await subscription.unsubscribe();
  window.localStorage.removeItem("runnarrPushSubscriptionId");
}

async function notificationServiceWorkerRegistration() {
  const current = await navigator.serviceWorker.getRegistration();
  if (current) return current;
  return navigator.serviceWorker.register("/sw.js?v=3", { updateViaCache: "none" });
}

function decodeApplicationServerKey(value: string) {
  const padding = "=".repeat((4 - value.length % 4) % 4);
  const raw = window.atob((value + padding).replace(/-/g, "+").replace(/_/g, "/"));
  return Uint8Array.from(raw, (character) => character.charCodeAt(0));
}

function defaultDeviceName() {
  const navigatorWithHints = navigator as Navigator & { userAgentData?: { platform?: string } };
  const platform = navigatorWithHints.userAgentData?.platform || navigator.platform || "Browser";
  return `${platform} device`;
}

function updateAppBadge(unreadCount?: number) {
  const badgeNavigator = navigator as Navigator & { setAppBadge?: (count?: number) => Promise<void>; clearAppBadge?: () => Promise<void> };
  if (typeof unreadCount !== "number") return;
  if (unreadCount > 0) {
    void badgeNavigator.setAppBadge?.(unreadCount).catch(() => undefined);
  } else {
    void badgeNavigator.clearAppBadge?.().catch(() => undefined);
  }
}

export function safeNotificationActionPath(path: string, origin = window.location.origin) {
  try {
    const parsed = new URL(path, origin);
    return parsed.origin === origin && path.startsWith("/") && !path.startsWith("//") ? `${parsed.pathname}${parsed.search}${parsed.hash}` : "/notifications";
  } catch {
    return "/notifications";
  }
}
