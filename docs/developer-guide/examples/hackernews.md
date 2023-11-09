# Hacker News

Let's write a hackernews extension using deno. Deno is an alternative runtime for javascript and typescript, which among other features allows you to easily write single-file scripts that use external dependencies.

We will use the `#!/usr/bin/env -S deno run -A` shebang to run our script using deno. The `-S` splits the command into arguments.

```ts
#!/usr/bin/env deno run -A

import Parser from "npm:rss-parser";
import { formatDistance } from "npm:date-fns";
import * as sunbeam from "npm:sunbeam-types@0.23.15"

if (Deno.args.length == 0) {
    const manifest: sunbeam.Manifest = {
        title: "Hacker News",
        root: [
            {
                title: "Front Page",
                command: "browse",
                params: {
                    topic: "frontpage"
                }
            },
            {
                title: "Show HN",
                command: "browse",
                params: {
                    topic: "show"
                }
            }
        ],
        commands: [
            {
                name: "browse",
                title: "Show a feed",
                mode: "list",
                params: [
                    { name: "topic", description: "Topic", required: true, type: "string" }
                ],
            },
        ]
    };

    console.log(JSON.stringify(manifest));
    Deno.exit(0);
}

const payload = JSON.parse(Deno.args[0]) as sunbeam.CommandInput;
if (payload.command == "browse") {
    const { topic } = payload.params as { topic: string };
    const feed = await new Parser().parseURL(`https://hnrss.org/${topic}?description=0&count=25`);
    const page: sunbeam.List = {
        items: feed.items.map((item) => ({
            title: item.title || "",
            subtitle: item.categories?.join(", ") || "",
            accessories: item.isoDate
                ? [
                    formatDistance(new Date(item.isoDate), new Date(), {
                        addSuffix: true,
                    }),
                ]
                : [],
            actions: [
                {
                    title: "Open in browser",
                    type: "open",
                    target: item.link || "",
                    exit: true
                },
                {
                    title: "Open Comments in Browser",
                    type: "open",
                    target: item.guid || "",
                    exit: true
                },
                {
                    title: "Copy Link",
                    type: "copy",
                    key: "c",
                    text: item.link || "",
                    exit: true
                },
            ],
        })),
    };

    console.log(JSON.stringify(page));
} else {
    console.error("Unknown command");
    Deno.exit(1);
}
```