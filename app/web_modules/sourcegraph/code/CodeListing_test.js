import autotest from "sourcegraph/util/autotest";

import React from "react";

import CodeListing from "sourcegraph/code/CodeListing";

import testdataLines from "sourcegraph/code/testdata/CodeListing-lines.json";
import testdataNoLineNumbers from "sourcegraph/code/testdata/CodeListing-noLineNumbers.json";

describe("CodeListing", () => {
	it("should render lines", () => {
		autotest(testdataLines, `${__dirname}/testdata/CodeListing-lines.json`,
			<CodeListing contents={"hello\nworld"} lineNumbers={true} startLine={1} endLine={2} highlightedDef="otherDef" />
		);
	});

	it("should not render line numbers by default", () => {
		autotest(testdataNoLineNumbers, `${__dirname}/testdata/CodeListing-noLineNumbers.json`,
			<CodeListing contents={"hello\nworld"} highlightedDef={null} />
		);
	});
});
