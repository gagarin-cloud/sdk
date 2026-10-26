/**
 * A size a job run can be given. `"g"` is the GPU size: one L4 GPU (24 GB) with
 * 6 vCPU and 40 GiB, dedicated, for jobs only. Every run at `"g"` is billed its
 * length plus a 15-minute start-up charge, and waits pending for three to six
 * minutes while the GPU node boots. The union is shared with anything else that
 * names a size, but the API accepts `"g"` on a run only and refuses it on a
 * service deploy or a resource with `invalid_size` (services take `s`, `m`, `l`).
 */
export type Size = "s" | "m" | "l" | "g";
export type Phase = "pending" | "running" | "done" | "failed" | "suspended";

/** Who the credential is, what it may do, and where gagarin puts things. */
export interface Whoami {
  /** The account's email address. */
  account: string;
  account_created_at: string;
  /** The name the credential was issued to, e.g. "gg on laptop". */
  client: string;
  /** The credential's capabilities: "deploy", "destroy", both or neither. */
  can: string[];
  /** What generated service addresses end in. */
  base_domain: string;
  /** The registry host images are run from. */
  registry: string;
  /** The cluster's OS/architecture, e.g. "linux/amd64": what an image must be built for. */
  platform: string;
}

/** Where the account stands. Money is in micro-USD, with a formatted string beside the figures a person reads. */
export interface Billing {
  micro_usd: number;
  balance: string;
  burn_per_hour_micro_usd: number;
  /** Null when nothing is burning: the balance lasts forever. */
  runway_hours: number | null;
  has_card: boolean;
  auto_top_up: boolean;
  suspended: boolean;
  top_up_at: string;
  top_up_for: string;
  top_up_at_micro_usd: number;
  top_up_for_micro_usd: number;
  /** How many services are on the meter now. */
  services: number;
  memories: number;
}

export interface Project {
  id: string;
  name: string;
  created_at: string;
  /** The caller's role: "owner", "editor" or "viewer". */
  role: string;
  suspended_at?: string;
  suspended_reason?: string;
  account_suspended_at?: string;
}

/** A service, job or resource as stored: what it was asked to be. */
export interface Service {
  project: string;
  name: string;
  /** "container" for a service, "job" for a job, or a resource's type ("postgres", ...). */
  kind: string;
  size?: string;
  image: string;
  digest?: string;
  port: number;
  volume_size_gb?: number;
  timeout_seconds?: number;
  /** A job's; absent when it is the default, 28800 (8 hours). */
  ttl_seconds?: number;
  schedule?: string;
  time_zone?: string;
  custom_domains?: string[];
  updated_by?: string;
  created_at: string;
  updated_at: string;
}

export interface RunState {
  revision: number;
  phase: Phase;
  exit_code?: number;
  started_at?: string;
  finished_at?: string;
  /** A scheduled firing whose TTL ran out: no longer in the cluster; this is the platform's record of it. */
  expired?: boolean;
}

export interface ScheduleState {
  next_at?: string;
  last_scheduled_at?: string;
  suspended?: boolean;
}

/** What the cluster is really running. */
export interface Actual {
  exists: boolean;
  ready_replicas: number;
  desired_replicas: number;
  superseded_replicas?: number;
  running_image?: string;
  running_port?: number;
  ingress_exists: boolean;
  message?: string;
  stalled?: boolean;
  /** A job's latest run; absent for anything else, and before the first. */
  run?: RunState;
  /** A scheduled job's schedule; absent for anything else. */
  schedule?: ScheduleState;
}

export interface Domain {
  url: string;
  domain: string;
  generated: boolean;
  state: string;
  blocked_on?: string;
  sentence: string;
  dns?: Record<string, string>;
}

/** One row of a project's status. */
export interface ServiceStatus extends Service {
  actual: Actual;
  /** What this service may reach. */
  needs: string[];
  /** Whether the cluster matches what was asked for. */
  in_sync: boolean;
  /** Every address it answers on, custom first, generated last. */
  domains: Domain[];
  sentence: string;
  /** Names of its environment variables, never the values. */
  env_keys: string[];
  volume_usage?: { used_bytes: number; capacity_bytes: number };
  object_usage?: { used_bytes: number; measured_at: string };
  backups_enabled: boolean;
  restore?: Record<string, unknown>;
}

/** What a project is asked to run, next to what is actually running. */
export interface ProjectStatus {
  project: string;
  project_id: string;
  services: ServiceStatus[];
  platform: { reconciler_last_run?: string; converging: boolean };
  /** Sentences a reader should be told before the table. Empty when there is nothing. */
  notices: string[];
  usage_today: {
    services: { name: string; minutes: number; micro_usd: number }[];
    memory: { memories: number; micro_usd: number };
    micro_usd: number;
  };
}

/** One recorded deploy or run. Environment values are never returned, only names. */
export interface Deployment {
  revision: number;
  project: string;
  service: string;
  image: string;
  digest?: string;
  port: number;
  public: boolean;
  url?: string;
  needs: string[];
  env_keys: string[];
  volume_path?: string;
  volume_size_gb?: number;
  timeout_seconds?: number;
  ttl_seconds?: number;
  schedule?: string;
  time_zone?: string;
  deployed_by?: string;
  restored_from?: number;
  created_at: string;
}

export interface LogsOptions {
  /** How many lines, newest last. */
  limit?: number;
  /** A duration back from now (`30m`, `6h`, `2d`) or an RFC 3339 time. */
  since?: string;
  /** As `since`; usually the `next` of the previous page. */
  until?: string;
  /** Keep only lines containing this text. */
  contains?: string;
  /** Read the last crashed container's output instead. */
  previous?: boolean;
}

export interface Logs {
  /** The lines, oldest first, newline-terminated. */
  logs: string;
  source: "store" | "cluster";
  /** The `until` that reads the page before this one, when there is one. */
  next: string | null;
  notice?: string;
}

export interface Check {
  ok: boolean;
  detail?: string;
  ms: number;
}

export interface Health {
  ok: boolean;
  serving: Check;
  cluster?: Check;
  reconciler?: Check;
}
