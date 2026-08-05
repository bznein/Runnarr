export function trainingSheetSourceURL(sourceUrl?: string): string | undefined {
  const value = sourceUrl?.trim();
  if (!value) {
    return undefined;
  }

  try {
    const parsed = new URL(value);
    if (parsed.protocol !== "https:" || parsed.username || parsed.password) {
      return undefined;
    }
    return parsed.href;
  } catch {
    return undefined;
  }
}
