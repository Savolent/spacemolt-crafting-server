-- Migration 008: Remove crafting gates and quality system (v0.226.0)
--
-- The game server consolidated skills from 139 to 28, removed recipe-level
-- skill requirements ("crafting gates"), and removed the quality system.
-- Skills now affect batch size and bonus output rather than gating access.

-- Drop the recipe_skills table (crafting gates removed)
DROP TABLE IF EXISTS recipe_skills;

-- Remove quality and skill columns from recipes.
-- SQLite 3.35.0+ supports ALTER TABLE DROP COLUMN.
ALTER TABLE recipes DROP COLUMN base_quality;
ALTER TABLE recipes DROP COLUMN skill_quality_mod;
ALTER TABLE recipes DROP COLUMN required_skills;

-- Remove quality_mod from recipe_outputs.
ALTER TABLE recipe_outputs DROP COLUMN quality_mod;
