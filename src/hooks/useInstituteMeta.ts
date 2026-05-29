import { useState, useEffect } from "react";
import { apiJson } from "@/api/client";

interface InstituteMeta {
  serverNow: string | null;
  instituteTZ: string | null;
}

export default function useInstituteMeta(): InstituteMeta {
  const [meta, setMeta] = useState<InstituteMeta>({ serverNow: null, instituteTZ: null });

  useEffect(() => {
    apiJson<{ institute_tz: string; server_now: string }>("/api/v1/meta/time", { method: "GET" })
      .then((d) => setMeta({ serverNow: d.server_now, instituteTZ: d.institute_tz }))
      .catch(() => {});
  }, []);

  return meta;
}
