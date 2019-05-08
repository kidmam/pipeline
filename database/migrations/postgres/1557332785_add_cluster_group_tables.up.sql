CREATE SEQUENCE clustergroups_id_seq INCREMENT 1 MINVALUE 1 MAXVALUE 9223372036854775807 START 1 CACHE 1;

CREATE TABLE "public"."clustergroups" (
    "id" integer DEFAULT nextval('clustergroups_id_seq') NOT NULL,
    "uid" text,
    "created_at" timestamptz,
    "updated_at" timestamptz,
    "deleted_at" timestamptz,
    "created_by" integer,
    "name" text,
    "organization_id" integer,
    CONSTRAINT "clustergroups_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "idx_uid" UNIQUE ("uid"),
    CONSTRAINT "idx_unique_id" UNIQUE ("deleted_at", "name", "organization_id")
) WITH (oids = false);

CREATE INDEX "idx_clustergroups_deleted_at" ON "public"."clustergroups" USING btree ("deleted_at");

CREATE SEQUENCE clustergroup_features_id_seq INCREMENT 1 MINVALUE 1 MAXVALUE 9223372036854775807 START 1 CACHE 1;

CREATE TABLE "public"."clustergroup_features" (
    "id" integer DEFAULT nextval('clustergroup_features_id_seq') NOT NULL,
    "name" text,
    "cluster_group_id" integer,
    "enabled" boolean,
    "properties" json,
    CONSTRAINT "clustergroup_features_pkey" PRIMARY KEY ("id")
) WITH (oids = false);

CREATE SEQUENCE clustergroup_members_id_seq INCREMENT 1 MINVALUE 1 MAXVALUE 9223372036854775807 START 1 CACHE 1;

CREATE TABLE "public"."clustergroup_members" (
    "id" integer DEFAULT nextval('clustergroup_members_id_seq') NOT NULL,
    "cluster_group_id" integer,
    "cluster_id" integer,
    CONSTRAINT "clustergroup_members_pkey" PRIMARY KEY ("id")
) WITH (oids = false);

CREATE SEQUENCE clustergroup_deployments_id_seq INCREMENT 1 MINVALUE 1 MAXVALUE 9223372036854775807 START 1 CACHE 1;

CREATE TABLE "public"."clustergroup_deployments" (
    "id" integer DEFAULT nextval('clustergroup_deployments_id_seq') NOT NULL,
    "cluster_group_id" integer,
    "created_at" timestamptz,
    "updated_at" timestamptz,
    "deployment_name" text,
    "deployment_version" text,
    "deployment_package" bytea,
    "deployment_release_name" text,
    "description" text,
    "chart_name" text,
    "namespace" text,
    "organization_name" text,
    "values" text,
    CONSTRAINT "clustergroup_deployments_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "idx_unique_cid_name" UNIQUE ("cluster_group_id", "deployment_name")
) WITH (oids = false);

CREATE SEQUENCE clustergroup_deployment_target_clusters_id_seq INCREMENT 1 MINVALUE 1 MAXVALUE 9223372036854775807 START 1 CACHE 1;

CREATE TABLE "public"."clustergroup_deployment_target_clusters" (
    "id" integer DEFAULT nextval('clustergroup_deployment_target_clusters_id_seq') NOT NULL,
    "cluster_group_deployment_id" integer,
    "cluster_id" integer,
    "cluster_name" text,
    "created_at" timestamptz,
    "updated_at" timestamptz,
    "values" text,
    CONSTRAINT "clustergroup_deployment_target_clusters_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "idx_unique_dep_cl" UNIQUE ("cluster_group_deployment_id", "cluster_id")
) WITH (oids = false);
