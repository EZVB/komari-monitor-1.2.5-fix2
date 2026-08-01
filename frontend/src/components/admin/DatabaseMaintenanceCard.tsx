import { formatBytes } from "@/utils/unitHelper";
import { Button, Dialog, Flex, Text } from "@radix-ui/themes";
import { DatabaseZap, RefreshCw } from "lucide-react";
import React from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { z } from "zod";
import { SettingCard } from "./SettingCard";

const maintenanceActionSchema = z.enum([
  "vacuum",
  "optimize",
  "vacuum_full",
]);
const nullableSizeSchema = z.number().finite().nonnegative().nullable();
const databaseInfoSchema = z.object({
  driver: z.string().trim().min(1),
  location: z.enum(["local", "external"]),
  size: nullableSizeSchema,
  action: maintenanceActionSchema,
  error: z.string().optional(),
});
const databaseOverviewSchema = z.object({
  main: databaseInfoSchema,
  monitoring: databaseInfoSchema.optional(),
  local_total: nullableSizeSchema,
});
const legacyDatabaseOverviewSchema = z.object({
  type: z.string().trim().min(1),
  size: z.number().finite().nonnegative(),
});
const maintenanceItemSchema = z.object({
  driver: z.string().trim().min(1),
  action: maintenanceActionSchema,
  before: nullableSizeSchema,
  after: nullableSizeSchema,
  success: z.boolean(),
  error: z.string().optional(),
  size_error: z.string().optional(),
});
const maintenanceResultSchema = z.object({
  all_succeeded: z.boolean(),
  main: maintenanceItemSchema,
  monitoring: maintenanceItemSchema.optional(),
});
const legacyMaintenanceResultSchema = z.object({
  before: z.number().finite().nonnegative(),
  after: z.number().finite().nonnegative(),
  size: z.number().finite().nonnegative().optional(),
});

type DatabaseInfo = z.infer<typeof databaseInfoSchema>;
type DatabaseOverview = z.infer<typeof databaseOverviewSchema>;
type DatabaseMaintenanceResult = z.infer<typeof maintenanceResultSchema>;
type TranslationFunction = ReturnType<typeof useTranslation>["t"];

function maintenanceActionForDriver(
  driver: string,
): z.infer<typeof maintenanceActionSchema> {
  switch (driver.toLowerCase()) {
    case "mysql":
    case "mariadb":
      return "optimize";
    case "postgres":
    case "postgresql":
      return "vacuum_full";
    default:
      return "vacuum";
  }
}

function parseDatabaseOverview(value: unknown): DatabaseOverview | null {
  const current = databaseOverviewSchema.safeParse(value);
  if (current.success) return current.data;

  const legacy = legacyDatabaseOverviewSchema.safeParse(value);
  if (!legacy.success) return null;

  const driver = legacy.data.type;
  const local = driver.toLowerCase() === "sqlite";
  return {
    main: {
      driver,
      location: local ? "local" : "external",
      size: local ? legacy.data.size : null,
      action: maintenanceActionForDriver(driver),
    },
    local_total: local ? legacy.data.size : null,
  };
}

function parseMaintenanceResult(
  value: unknown,
  overview: DatabaseOverview,
): DatabaseMaintenanceResult | null {
  const current = maintenanceResultSchema.safeParse(value);
  if (current.success) return current.data;

  const legacy = legacyMaintenanceResultSchema.safeParse(value);
  if (!legacy.success) return null;

  return {
    all_succeeded: true,
    main: {
      driver: overview.main.driver,
      action: overview.main.action,
      before: legacy.data.before,
      after: legacy.data.after,
      success: true,
    },
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

async function requestAdminData(
  input: RequestInfo | URL,
  fallbackMessage: string,
  init?: RequestInit,
): Promise<unknown> {
  const response = await fetch(input, init);
  let payload: unknown;

  try {
    payload = await response.json();
  } catch {
    throw new Error(fallbackMessage);
  }

  const message =
    isRecord(payload) && typeof payload.message === "string"
      ? payload.message
      : fallbackMessage;
  if (
    !response.ok ||
    !isRecord(payload) ||
    payload.status !== "success" ||
    !("data" in payload)
  ) {
    throw new Error(message);
  }

  return payload.data;
}

function driverLabel(driver: string): string {
  switch (driver.toLowerCase()) {
    case "sqlite":
      return "SQLite";
    case "mysql":
    case "mariadb":
      return "MySQL / MariaDB";
    case "postgres":
    case "postgresql":
      return "PostgreSQL";
    default:
      return driver;
  }
}

function DatabaseSummaryRow({ label, info }: { label: string; info: DatabaseInfo }) {
  const { t } = useTranslation();

  return (
    <div className="grid w-full grid-cols-[minmax(0,1fr)_auto] items-start gap-x-4 gap-y-1 border-b border-[var(--gray-a5)] py-3">
      <div className="min-w-0">
        <Text as="div" size="2" weight="medium">
          {label}
        </Text>
        <Text as="div" size="1" color="gray">
          {driverLabel(info.driver)} / {t(`settings.database.locations.${info.location}`)}
        </Text>
        {info.error ? (
          <Text as="div" size="1" color="red" className="break-words">
            {info.error}
          </Text>
        ) : null}
      </div>
      <Text size="2" weight="medium" className="whitespace-nowrap">
        {info.size === null ? t("common.unknown") : formatBytes(info.size)}
      </Text>
    </div>
  );
}

function LocalDatabaseTotalRow({ size }: { size: number }) {
  const { t } = useTranslation();

  return (
    <div className="grid w-full grid-cols-[minmax(0,1fr)_auto] items-start gap-x-4 py-3">
      <div className="min-w-0">
        <Text as="div" size="2" weight="medium">
          {t("settings.database.local_total")}
        </Text>
        <Text as="div" size="1" color="gray">
          {t("settings.database.local_total_description")}
        </Text>
      </div>
      <Text size="2" weight="bold" className="whitespace-nowrap">
        {formatBytes(size)}
      </Text>
    </div>
  );
}

function MaintenanceActionRow({
  label,
  info,
}: {
  label: string;
  info: DatabaseInfo;
}) {
  const { t } = useTranslation();

  return (
    <div className="grid grid-cols-1 gap-1 border-b border-[var(--gray-a5)] py-3 last:border-b-0 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] sm:gap-4">
      <div className="min-w-0">
        <Text as="div" size="2" weight="medium">
          {label}
        </Text>
        <Text as="div" size="1" color="gray">
          {driverLabel(info.driver)} / {t(`settings.database.locations.${info.location}`)}
        </Text>
      </div>
      <Text
        as="div"
        size="2"
        weight="medium"
        className="break-words sm:text-right"
      >
        {t(`settings.database.actions.${info.action}`)}
      </Text>
    </div>
  );
}

function maintenanceFailureDescription(
  result: DatabaseMaintenanceResult,
  t: TranslationFunction,
): string | undefined {
  const items: Array<[string, z.infer<typeof maintenanceItemSchema>]> = [
    [t("settings.database.main"), result.main],
  ];
  if (result.monitoring) {
    items.push([t("settings.database.monitoring"), result.monitoring]);
  }
  const failures = items
    .filter(([, item]) => !item.success)
    .map(
      ([label, item]) =>
        `${label}: ${
          item.error || item.size_error || t("settings.database.operation_failed")
        }`,
    );

  return failures.length > 0 ? failures.join("; ") : undefined;
}

export function DatabaseMaintenanceCard() {
  const { t } = useTranslation();
  const [overview, setOverview] = React.useState<DatabaseOverview | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [loadError, setLoadError] = React.useState<string | null>(null);
  const [maintaining, setMaintaining] = React.useState(false);
  const [confirmOpen, setConfirmOpen] = React.useState(false);

  const fetchOverview = React.useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    const fallbackMessage = t("settings.database.load_error");

    try {
      const data = await requestAdminData(
        "/api/admin/database/size",
        fallbackMessage,
      );
      const parsed = parseDatabaseOverview(data);
      if (!parsed) {
        throw new Error(t("settings.database.invalid_response"));
      }

      setOverview(parsed);
      setLoadError(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setOverview(null);
      setLoadError(
        message === fallbackMessage
          ? fallbackMessage
          : `${fallbackMessage}: ${message}`,
      );
    } finally {
      setLoading(false);
    }
  }, [t]);

  React.useEffect(() => {
    void fetchOverview();
  }, [fetchOverview]);

  const handleMaintenance = async () => {
    if (!overview || maintaining) return;

    setConfirmOpen(false);
    setMaintaining(true);
    try {
      const data = await requestAdminData(
        "/api/admin/database/vacuum",
        t("settings.database.maintenance_error"),
        { method: "POST" },
      );
      const result = parseMaintenanceResult(data, overview);
      if (!result) {
        throw new Error(t("settings.database.invalid_response"));
      }

      const allItemsSucceeded =
        result.main.success && (result.monitoring?.success ?? true);
      if (result.all_succeeded && allItemsSucceeded) {
        toast.success(t("settings.database.maintenance_success"));
      } else {
        toast.warning(t("settings.database.maintenance_partial_failure"), {
          description: maintenanceFailureDescription(result, t),
        });
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      toast.error(t("settings.database.maintenance_error"), {
        description: message,
      });
    } finally {
      await fetchOverview();
      setMaintaining(false);
    }
  };

  const actionDisabled = loading || maintaining;

  return (
    <SettingCard
      title={t("settings.database.maintenance_title")}
      description={t("settings.database.maintenance_description")}
    >
      <Flex direction="column" className="w-full pt-2" gap="0">
        {overview ? (
          <>
            <DatabaseSummaryRow
              label={t("settings.database.main")}
              info={overview.main}
            />
            {overview.monitoring ? (
              <DatabaseSummaryRow
                label={t("settings.database.monitoring")}
                info={overview.monitoring}
              />
            ) : null}
            {overview.local_total !== null ? (
              <LocalDatabaseTotalRow size={overview.local_total} />
            ) : null}
          </>
        ) : loading ? (
          <Text size="2" color="gray" className="py-3">
            {t("loading")}
          </Text>
        ) : null}

        {loadError ? (
          <Text size="2" color="red" className="break-words py-3">
            {loadError}
          </Text>
        ) : null}

        <Flex justify="end" className="pt-3">
          {!overview && loadError ? (
            <Button
              variant="soft"
              disabled={loading}
              onClick={() => void fetchOverview()}
            >
              <RefreshCw
                size={16}
                className={loading ? "animate-spin" : undefined}
              />
              {t("common.retry")}
            </Button>
          ) : overview ? (
            <Dialog.Root open={confirmOpen} onOpenChange={setConfirmOpen}>
              <Dialog.Trigger>
                <Button
                  variant="solid"
                  color="orange"
                  disabled={actionDisabled}
                >
                  <DatabaseZap size={16} />
                  {maintaining
                    ? t("settings.database.maintaining")
                    : t("settings.database.maintenance_button")}
                </Button>
              </Dialog.Trigger>
              <Dialog.Content maxWidth="520px">
                <Dialog.Title>
                  {t("settings.database.confirm_title")}
                </Dialog.Title>
                <Dialog.Description size="2">
                  {t("settings.database.confirm_description")}
                </Dialog.Description>

                <Flex direction="column" mt="3">
                  <MaintenanceActionRow
                    label={t("settings.database.main")}
                    info={overview.main}
                  />
                  {overview.monitoring ? (
                    <MaintenanceActionRow
                      label={t("settings.database.monitoring")}
                      info={overview.monitoring}
                    />
                  ) : null}
                </Flex>

                <Flex gap="3" mt="4" justify="end">
                  <Dialog.Close>
                    <Button variant="soft" color="gray">
                      {t("common.cancel")}
                    </Button>
                  </Dialog.Close>
                  <Button
                    variant="solid"
                    color="orange"
                    onClick={() => void handleMaintenance()}
                  >
                    <DatabaseZap size={16} />
                    {t("settings.database.maintenance_button")}
                  </Button>
                </Flex>
              </Dialog.Content>
            </Dialog.Root>
          ) : null}
        </Flex>
      </Flex>
    </SettingCard>
  );
}
