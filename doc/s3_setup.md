# S3 Recommended Setup # 

By default all resources in S3 are private to the account. While you can assign
ACLs explicitly to allow other people access, you have to do this with each
upload which can be awkward. It's actually much easier to share resources with
a specific team by using a main organisation Amazon account, then creating an
AWS Identity and Access Management (IAM) user in your AWS account.

This way not only do these sub-accounts have access to your S3 bucket, but any
objects they create are automatically owned by the root account. This means
you always have control over the objects created by the team from this root
account.

[Amazon's documentation on access control](https://docs.aws.amazon.com/AmazonS3/latest/dev/access-control-overview.html)

## Create a bucket ##

First, just create a bucket in your root S3 account the usual way. 

## Create a group ##

1. Open the AWS console as the root account
2. Click Identity and Access Management
3. Click Groups in the sidebar
4. Click Create New Group
5. Give it a name appropriate for read/write access e.g. 'git-lob.rw'
6. If you want to give the group access to ALL buckets, scroll down and check 
   the box next to AmazonS3FullAccess & click Next
   If you want to give the group access to only specific buckets, don't check
   any boxes, just click Next
7. Proceed to confirm the group

## Give specific bucket permissions to the group ##

If you didn't grant access to every bucket in step 6 above, you need to add an
Inline Policy in the group details (at the bottom of the group details) to allow
this group to have permissions to your git-lob bucket.

1. Create an Inline Policy in the bottom section
2. Select Policy Generator
3. Effect: Allow
   AWS Service: Amazon S3
   Actions: All Actions
   Amazon Resource Name (ARN): arn:aws:s3:::*[bucket_name]*
4. Click Add Statement then Next
5. Give it a meaningful name e.g. 'Readwrite_git-lob' then Apply Policy 

##Create read-only permissions if you want ##

If you want to, you can either open permissions read-only to the bucket to 
everyone, or you can repeat the steps above to give read-only access to 
another group that you create. 

## Create users ##

You want to create a user underneath this root account for everyone who will
be granted access - including you. You should use your user credentials
rather than your root credentials in all normal usage.

1. Back at the Identiry and Access Management root, click Users in the sidebar
2. Enter up to 5 user names and keep the 'Generate access key' checkbox enabled
3. Click Create, then make a note of the key pairs for these users 
4. Distribute the key pairs securely to the users in question
5. Go back to the Groups section in the sidebar and assign the users to the
   read/write group you created earlier (or the read-only group if you added 
   one)

## Choosing what credentials to use at runtime ##

There's a good chance that at least one of your users will end up having more
than one S3 account so will need to choose which one to use. To manage this, 
create multiple profiles in your AWS configuration files, stored in 
~/.aws/ (config and credentials files). Both files can have sections for 
different profiles, the default settings being under [default]

You can choose which profile to use at any given time multiple ways:

1. Set it per remote in .git/config:
   git config remote.*[remote_name]*.git-lob-s3-profile *[profile_name]*
2. Set it per repo or globally for git only
   git config git-lob.s3-profile *[profile_name]*
3. Set AWS_PROFILE in your environment

So you could put the new credentials for this user into a new section of your
~/.aws/credentials file, then set your git config (per remote, per repo or
globally) to specify this profile, thus using the correct setup without
affecting any other usage of S3 you have.

