import { zh as quickConnectMessages } from '@/custom/vote-ai/quick-connect/messages'
import { startPageMessages } from '@/custom/vote-ai/quick-connect/page-messages'
import { zh as quickCreateKey } from '@/custom/vote-ai/quick-connect/create-key-messages'
import { entryMessages } from '@/custom/vote-ai/quick-connect/entry-messages'
import { journeyMessages } from '@/custom/vote-ai/quick-connect/journey-messages'
import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import batchImage from './batchImage'
import admin from './admin'
import misc from './misc'

export default {
  firstUseJourney: journeyMessages.zh,
  rikkaHub: rikkaHubMessages.zh,
  quickConnect: { ...quickConnectMessages, startPage: startPageMessages.zh, entryFlow: entryMessages.zh },
  quickCreateKey,
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...batchImage,
  admin,
  ...misc,
}
import rikkaHubMessages from '@/custom/vote-ai/rikkahub/messages'
