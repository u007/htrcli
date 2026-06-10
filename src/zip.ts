import { readFileSync } from "node:fs";
import * as gulp from "gulp";
import file from "gulp-zip";

const _manifest = JSON.parse(
	readFileSync("./build/manifest.json", "utf-8"),
) as {
	name: string;
	version: string;
};

gulp
	.src("build/**", { encoding: false })
	.pipe(file("how-to-recorder.zip"))
	.pipe(gulp.dest("."));
