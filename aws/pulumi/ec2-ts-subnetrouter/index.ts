import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";
import * as awsx from "@pulumi/awsx";
import * as fs from 'fs';
import * as cloudinit from '@pulumi/cloudinit';

const config = new pulumi.Config();
const tailscaleOauthClientSecret = config.requireSecret("tailscaleOauthClientSecret");

// VPC for things to live in
const vpc = new awsx.ec2.Vpc("lbr-pulumi-example", {
    cidrBlock: "172.16.0.0/16",
    numberOfAvailabilityZones: 2,
    subnetStrategy: awsx.ec2.SubnetAllocationStrategy.Auto,
    subnetSpecs: [
        { name: "public", type: "Public" },
        { name: "private", type: "Private" },
    ],
    enableDnsHostnames: true,
    enableDnsSupport: true,
})

// IAM role
const iamRole = new aws.iam.Role("lbr-pulumi-example", {
    assumeRolePolicy: aws.iam.assumeRolePolicyForPrincipal({
        Service: "ec2.amazonaws.com",
    }),
})

// attach SSM managed instance for debugging
const managedPolicyArns: string[] = [
    'arn:aws:iam::aws:policy/AmazonEC2FullAccess',
    'arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore'
]

/*
  Loop through the managed policies and attach
  them to the defined IAM role
*/
let counter = 0;
for (const policy of managedPolicyArns) {
    // Create RolePolicyAttachment without returning it.
    const rpa = new aws.iam.RolePolicyAttachment(`example-policy-${counter++}`,
        { policyArn: policy, role: iamRole.id }, { parent: iamRole }
    );
}

/*
  Allow logging etc to cloudwatch
*/
const cloudwatchPolicy = new aws.iam.RolePolicy('lbr-pulumi-example', {
    role: iamRole.id,
    policy: {
        Version: "2012-10-17",
        Statement: [{
            Action: [
                "cloudwatch:PutMetricData",
                "ec2:DescribeVolumes",
                "ec2:DescribeTags",
                "logs:PutLogEvents",
                "logs:DescribeLogStreams",
                "logs:DescribeLogGroups",
                "logs:CreateLogStream",
                "logs:CreateLogGroup"
            ],
            Effect: "Allow",
            Resource: "*",
        }],
    },
});

const instanceProfile = new aws.iam.InstanceProfile('lbr-pulumi-example', {
    role: iamRole.name
})

const ami = aws.ec2.getAmi({
    mostRecent: true,
    filters: [
        {
            name: "name",
            values: ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"],
        },
        {
            name: "virtualization-type",
            values: ["hvm"],
        },
    ],
    owners: ["099720109477"], // Canonical
});

// Define a function to render templates correctly
function renderTemplate(templatePath: string, variables: Record<string, any>): pulumi.Output<string> {
    // Read the template file as a string
    const template = fs.readFileSync(templatePath, "utf-8");

    // Handle Pulumi Outputs
    const resolvedVariables = pulumi.all(variables).apply((resolved) => {
        return Object.entries(resolved).reduce((result, [key, value]) => {
            const regex = new RegExp(`\\$\\{${key}\\}`, "g"); // Matches Terraform-style ${var_name}
            return result.replace(regex, value.toString());
        }, template);
    });

    return resolvedVariables;
}

// the install Tailscale template
const installTailscaleTmpl = renderTemplate("./templates/install_tailscale.sh.tmpl", {
    TRACK: "stable",
    MAX_RETRIES: 5,
    RETRY_DELAY: 10,
});

// the set up Tailscale template
const setupTailscaleTmpl = renderTemplate("./templates/setup_tailscale.sh.tmpl", {
    HOSTNAME: "lbr-pulumi-example",
    ADVERTISE_TAGS: ["tag:example"],
    AUTH_KEY: tailscaleOauthClientSecret,
});

// Log the setup Tailscale template
// setupTailscaleTmpl.apply((rendered) => {
//     console.log("Rendered setup Tailscale template:");
//     console.log(rendered);
// });


// read the static file
const ipForwardingContent = fs.readFileSync("./files/ip_forwarding.sh", "utf-8");


// Turn the above templates into cloud-init data
const userData = cloudinit.getConfigOutput({
    gzip: false,
    base64Encode: true,
    parts: [
        {
            contentType: "text/x-shellscript",
            content: installTailscaleTmpl,
        },
        {
            contentType: "text/x-shellscript",
            content: setupTailscaleTmpl,
        },
        {
            contentType: "text/x-shellscript",
            content: ipForwardingContent,
        }
    ],
})

const instanceSecurityGroups = new aws.ec2.SecurityGroup('example-instance-securitygroup', {
    vpcId: vpc.vpcId,
    description: "Allow all ports from same subnet",
    ingress: [{
        protocol: '-1',
        fromPort: 0,
        toPort: 0,
        cidrBlocks: ["172.16.0.0/24"]
    }, {
        protocol: 'udp',
        fromPort: 41641,
        toPort: 41641,
        cidrBlocks: ['0.0.0.0/0']
    }],
    egress: [{
        protocol: '-1',
        fromPort: 0,
        toPort: 0,
        cidrBlocks: ['0.0.0.0/0'],
    }]
})

// create a launch template to be used for ASG
const launchTemplate = new aws.ec2.LaunchTemplate('lbr-pulumi-example', {
    imageId: ami.then(ami => ami.id),
    instanceType: "t3.small",
    namePrefix: "lbr-pulumi-example",
    networkInterfaces: [{
        deleteOnTermination: "true",
        securityGroups: [instanceSecurityGroups.id],
    }],
    monitoring: {
        enabled: true
    },
    iamInstanceProfile: {
        arn: instanceProfile.arn
    },
    blockDeviceMappings: [{
        deviceName: "/dev/xvda",
        ebs: {
            volumeSize: 8,
            deleteOnTermination: "true",
            volumeType: "gp2",
        }
    }],
    metadataOptions: {
        httpTokens: "required", // required for Tailscale's AWS account, not always required
        httpPutResponseHopLimit: 2, // required for Tailscale's AWS account, not always required
        httpEndpoint: "enabled", 
    },
    userData: userData.rendered,
})

// Create a self healing autoscaling group
const asg = new aws.autoscaling.Group('lbr-pulumi-example', {
    maxSize: 2,
    minSize: 1,
    desiredCapacity: 2,
    launchTemplate: {
        id: launchTemplate.id,
        version: launchTemplate.latestVersion.apply(v => v.toString())
    },
    instanceRefresh: {
        strategy: "Rolling",
        preferences: {
            minHealthyPercentage: 50,
        },
    },
    vpcZoneIdentifiers: vpc.publicSubnetIds,
})


